package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
)

const schemaCacheTTL = 5 * time.Minute

var (
	ErrInvalidSchema      = errors.New("invalid collection schema")
	ErrCollectionExists   = errors.New("collection already exists")
	ErrCollectionNotFound = errors.New("collection not found")
)

// CollectionService manages dynamic PostgreSQL tables and their metadata in _collections.
type CollectionService struct {
	pool  *pgxpool.Pool
	cache cache.CacheStore
}

// NewCollectionService constructs a CollectionService backed by PostgreSQL and CacheStore.
func NewCollectionService(pool *pgxpool.Pool, c cache.CacheStore) *CollectionService {
	return &CollectionService{
		pool:  pool,
		cache: c,
	}
}

// Create validates the schema, executes CREATE TABLE, and registers metadata atomically.
func (s *CollectionService) Create(ctx context.Context, req models.CreateCollectionRequest) (*models.Collection, error) {
	ddl, err := buildCreateTableSQL(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}

	fieldsJSON, err := json.Marshal(req.Fields)
	if err != nil {
		return nil, fmt.Errorf("marshal fields: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Create the physical PostgreSQL table.
	if _, err := tx.Exec(ctx, ddl); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P07" { // duplicate_table
			return nil, ErrCollectionExists
		}
		return nil, fmt.Errorf("execute create table: %w", err)
	}

	// 2. Register schema metadata in _collections.
	var col models.Collection
	var rawFields []byte
	insertSQL := `
		INSERT INTO _collections (name, fields)
		VALUES ($1, $2)
		RETURNING id, name, fields, created_at, updated_at`

	err = tx.QueryRow(ctx, insertSQL, req.Name, fieldsJSON).
		Scan(&col.ID, &col.Name, &rawFields, &col.CreatedAt, &col.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, ErrCollectionExists
		}
		return nil, fmt.Errorf("insert _collections: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	_ = json.Unmarshal(rawFields, &col.Fields)
	_ = s.cache.Delete(ctx, schemaCacheKey(col.Name))
	return &col, nil
}

// List returns all registered collections ordered by ID.
func (s *CollectionService) List(ctx context.Context) ([]models.Collection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, fields, created_at, updated_at
		FROM _collections
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query collections: %w", err)
	}
	defer rows.Close()

	result := make([]models.Collection, 0)
	for rows.Next() {
		var col models.Collection
		var rawFields []byte
		if err := rows.Scan(&col.ID, &col.Name, &rawFields, &col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		_ = json.Unmarshal(rawFields, &col.Fields)
		result = append(result, col)
	}
	return result, rows.Err()
}

// GetByName fetches a collection schema, checking CacheStore before querying PostgreSQL.
func (s *CollectionService) GetByName(ctx context.Context, name string) (*models.Collection, error) {
	if !models.IsValidIdentifier(name) {
		return nil, ErrCollectionNotFound
	}

	// Fast path: check cache first.
	if cached, err := s.cache.Get(ctx, schemaCacheKey(name)); err == nil {
		var col models.Collection
		if json.Unmarshal([]byte(cached), &col) == nil {
			return &col, nil
		}
	}

	// Slow path: query PostgreSQL and populate cache.
	var col models.Collection
	var rawFields []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, fields, created_at, updated_at
		FROM _collections
		WHERE name = $1`, name).
		Scan(&col.ID, &col.Name, &rawFields, &col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCollectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	_ = json.Unmarshal(rawFields, &col.Fields)
	if encoded, err := json.Marshal(col); err == nil {
		_ = s.cache.Set(ctx, schemaCacheKey(name), string(encoded), schemaCacheTTL)
	}
	return &col, nil
}

// Delete drops the collection's PostgreSQL table and removes its metadata atomically.
func (s *CollectionService) Delete(ctx context.Context, name string) error {
	if !models.IsValidIdentifier(name) {
		return ErrCollectionNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `DELETE FROM _collections WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete _collections: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCollectionNotFound
	}

	dropSQL := fmt.Sprintf(`DROP TABLE IF EXISTS "%s" CASCADE`, name)
	if _, err := tx.Exec(ctx, dropSQL); err != nil {
		return fmt.Errorf("drop table: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	_ = s.cache.Delete(ctx, schemaCacheKey(name))
	return nil
}

func schemaCacheKey(name string) string {
	return "schema:" + name
}
