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

// Create validates the schema, executes CREATE TABLE + CREATE INDEX, and registers metadata atomically.
func (s *CollectionService) Create(ctx context.Context, req models.CreateCollectionRequest) (*models.Collection, error) {
	ddl, err := buildCreateTableSQL(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}

	indexStmts := make([]string, 0, len(req.Indexes))
	for _, idx := range req.Indexes {
		stmt, err := buildCreateIndexSQL(req.Name, idx)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
		}
		indexStmts = append(indexStmts, stmt)
	}

	fieldsJSON, err := json.Marshal(req.Fields)
	if err != nil {
		return nil, fmt.Errorf("marshal fields: %w", err)
	}
	indexes := req.Indexes
	if indexes == nil {
		indexes = []models.IndexDef{}
	}
	indexesJSON, err := json.Marshal(indexes)
	if err != nil {
		return nil, fmt.Errorf("marshal indexes: %w", err)
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

	// 2. Create any configured indexes / unique constraints.
	for _, stmt := range indexStmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("create index: %w", err)
		}
	}

	apiRules := req.APIRules
	if apiRules == nil {
		apiRules = map[string]string{}
	}
	rulesJSON, _ := json.Marshal(apiRules)

	// 3. Register schema metadata in _collections.
	var col models.Collection
	var rawFields, rawRules, rawIndexes []byte
	insertSQL := `
		INSERT INTO _collections (name, fields, api_rules, indexes)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, fields, api_rules, indexes, created_at, updated_at`

	err = tx.QueryRow(ctx, insertSQL, req.Name, fieldsJSON, rulesJSON, indexesJSON).
		Scan(&col.ID, &col.Name, &rawFields, &rawRules, &rawIndexes, &col.CreatedAt, &col.UpdatedAt)
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
	_ = json.Unmarshal(rawRules, &col.APIRules)
	_ = json.Unmarshal(rawIndexes, &col.Indexes)
	_ = s.cache.Delete(ctx, schemaCacheKey(col.Name))
	return &col, nil
}

// List returns all registered collections ordered by ID.
func (s *CollectionService) List(ctx context.Context) ([]models.Collection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, fields, api_rules, indexes, created_at, updated_at
		FROM _collections
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query collections: %w", err)
	}
	defer rows.Close()

	result := make([]models.Collection, 0)
	for rows.Next() {
		var col models.Collection
		var rawFields, rawRules, rawIndexes []byte
		if err := rows.Scan(&col.ID, &col.Name, &rawFields, &rawRules, &rawIndexes, &col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		_ = json.Unmarshal(rawFields, &col.Fields)
		_ = json.Unmarshal(rawRules, &col.APIRules)
		_ = json.Unmarshal(rawIndexes, &col.Indexes)
		result = append(result, col)
	}
	return result, rows.Err()
}

// GetByName fetches a collection schema, checking CacheStore before querying PostgreSQL.
func (s *CollectionService) GetByName(ctx context.Context, name string) (*models.Collection, error) {
	if !models.IsValidIdentifier(name) {
		return nil, ErrCollectionNotFound
	}

	if cached, err := s.cache.Get(ctx, schemaCacheKey(name)); err == nil {
		var col models.Collection
		if json.Unmarshal([]byte(cached), &col) == nil {
			return &col, nil
		}
	}

	var col models.Collection
	var rawFields, rawRules, rawIndexes []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, fields, api_rules, indexes, created_at, updated_at
		FROM _collections
		WHERE name = $1`, name).
		Scan(&col.ID, &col.Name, &rawFields, &rawRules, &rawIndexes, &col.CreatedAt, &col.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCollectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	_ = json.Unmarshal(rawFields, &col.Fields)
	_ = json.Unmarshal(rawRules, &col.APIRules)
	_ = json.Unmarshal(rawIndexes, &col.Indexes)
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
	if name == "users" {
		return fmt.Errorf("%w: built-in auth collection users cannot be deleted", ErrInvalidSchema)
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

func physicalTableName(name string) string {
	if name == "users" {
		return "_users"
	}
	return name
}

func schemaCacheKey(name string) string {
	return "schema:" + name
}

// Update alters the collection's PostgreSQL columns & indexes and updates metadata in _collections atomically.
func (s *CollectionService) Update(ctx context.Context, name string, newFields []models.FieldDef, apiRules map[string]string, newIndexes []models.IndexDef) (*models.Collection, error) {
	existing, err := s.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	tableName := physicalTableName(existing.Name)
	alterStmts, cleanedFields, err := buildAlterTableStatements(tableName, existing.Fields, newFields)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}

	if newIndexes == nil {
		newIndexes = existing.Indexes
	}
	if newIndexes == nil {
		newIndexes = []models.IndexDef{}
	}
	indexStmts, err := buildIndexDiffStatements(tableName, existing.Indexes, newIndexes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}

	fieldsJSON, err := json.Marshal(cleanedFields)
	if err != nil {
		return nil, fmt.Errorf("marshal fields: %w", err)
	}
	if apiRules == nil {
		apiRules = existing.APIRules
	}
	if apiRules == nil {
		apiRules = map[string]string{}
	}
	rulesJSON, _ := json.Marshal(apiRules)
	indexesJSON, _ := json.Marshal(newIndexes)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, stmt := range alterStmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("alter table: %w", err)
		}
	}
	for _, stmt := range indexStmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("update index: %w", err)
		}
	}

	var updated models.Collection
	var rawFields, rawRules, rawIndexes []byte
	err = tx.QueryRow(ctx, `
		UPDATE _collections
		SET fields = $2, api_rules = $3, indexes = $4, updated_at = NOW()
		WHERE name = $1
		RETURNING id, name, fields, api_rules, indexes, created_at, updated_at`,
		existing.Name, fieldsJSON, rulesJSON, indexesJSON,
	).Scan(&updated.ID, &updated.Name, &rawFields, &rawRules, &rawIndexes, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update _collections: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	_ = json.Unmarshal(rawFields, &updated.Fields)
	_ = json.Unmarshal(rawRules, &updated.APIRules)
	_ = json.Unmarshal(rawIndexes, &updated.Indexes)
	_ = s.cache.Delete(ctx, schemaCacheKey(updated.Name))
	return &updated, nil
}
