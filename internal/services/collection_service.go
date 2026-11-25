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

type CollectionService struct {
	pool  *pgxpool.Pool
	cache cache.CacheStore
}

func NewCollectionService(pool *pgxpool.Pool, c cache.CacheStore) *CollectionService {
	return &CollectionService{pool: pool, cache: c}
}

func normalizeCollectionType(t string) string {
	switch t {
	case "view", "auth":
		return t
	default:
		return "base"
	}
}

func (s *CollectionService) Create(ctx context.Context, req models.CreateCollectionRequest) (*models.Collection, error) {
	req.Type = normalizeCollectionType(req.Type)
	if req.IDType != "text" {
		req.IDType = "serial"
	}
	if req.IDType == "text" && req.IDAutogeneratePattern == "" {
		req.IDAutogeneratePattern = "[a-z0-9]{15}"
	}

	var ddl string
	var err error
	var indexStmts []string

	if req.Type == "view" {
		ddl, err = buildCreateViewSQL(req.Name, req.ViewQuery)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
		}
		req.Fields = []models.FieldDef{}
		req.Indexes = []models.IndexDef{}
	} else {
		ddl, err = buildCreateTableSQL(req)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
		}
		for _, idx := range req.Indexes {
			stmt, idxErr := buildCreateIndexSQL(req.Name, idx)
			if idxErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, idxErr)
			}
			indexStmts = append(indexStmts, stmt)
		}
	}

	if req.Fields == nil {
		req.Fields = []models.FieldDef{}
	}
	fieldsJSON, _ := json.Marshal(req.Fields)
	if req.Indexes == nil {
		req.Indexes = []models.IndexDef{}
	}
	indexesJSON, _ := json.Marshal(req.Indexes)
	apiRules := req.APIRules
	if apiRules == nil {
		apiRules = map[string]string{}
	}
	rulesJSON, _ := json.Marshal(apiRules)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, ddl); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P07" {
			return nil, ErrCollectionExists
		}
		return nil, fmt.Errorf("execute create %s: %w", req.Type, err)
	}

	for _, stmt := range indexStmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("create index: %w", err)
		}
	}

	var col models.Collection
	var rawFields, rawRules, rawIndexes []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO _collections (name, type, view_query, id_type, id_autogenerate_pattern, fields, api_rules, indexes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, type, view_query, id_type, id_autogenerate_pattern, fields, api_rules, indexes, created_at, updated_at`,
		req.Name, req.Type, req.ViewQuery, req.IDType, req.IDAutogeneratePattern, fieldsJSON, rulesJSON, indexesJSON,
	).Scan(&col.ID, &col.Name, &col.Type, &col.ViewQuery, &col.IDType, &col.IDAutogeneratePattern, &rawFields, &rawRules, &rawIndexes, &col.CreatedAt, &col.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
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

func (s *CollectionService) List(ctx context.Context) ([]models.Collection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, type, view_query, id_type, id_autogenerate_pattern, fields, api_rules, indexes, created_at, updated_at
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
		if err := rows.Scan(&col.ID, &col.Name, &col.Type, &col.ViewQuery, &col.IDType, &col.IDAutogeneratePattern, &rawFields, &rawRules, &rawIndexes, &col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		_ = json.Unmarshal(rawFields, &col.Fields)
		_ = json.Unmarshal(rawRules, &col.APIRules)
		_ = json.Unmarshal(rawIndexes, &col.Indexes)
		result = append(result, col)
	}
	return result, rows.Err()
}

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
		SELECT id, name, type, view_query, id_type, id_autogenerate_pattern, fields, api_rules, indexes, created_at, updated_at
		FROM _collections
		WHERE name = $1`, name).
		Scan(&col.ID, &col.Name, &col.Type, &col.ViewQuery, &col.IDType, &col.IDAutogeneratePattern, &rawFields, &rawRules, &rawIndexes, &col.CreatedAt, &col.UpdatedAt)
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

func (s *CollectionService) Delete(ctx context.Context, name string) error {
	if !models.IsValidIdentifier(name) {
		return ErrCollectionNotFound
	}
	if name == "users" {
		return fmt.Errorf("%w: built-in auth collection users cannot be deleted", ErrInvalidSchema)
	}

	existing, err := s.GetByName(ctx, name)
	if err != nil {
		return err
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
	if existing.Type == "view" {
		dropSQL = fmt.Sprintf(`DROP VIEW IF EXISTS "%s" CASCADE`, name)
	}
	if _, err := tx.Exec(ctx, dropSQL); err != nil {
		return fmt.Errorf("drop %s: %w", existing.Type, err)
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

func (s *CollectionService) Update(ctx context.Context, name string, newFields []models.FieldDef, apiRules map[string]string, newIndexes []models.IndexDef, viewQuery string) (*models.Collection, error) {
	existing, err := s.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	var alterStmts, indexStmts []string
	var cleanedFields []models.FieldDef

	if existing.Type == "view" {
		if viewQuery == "" {
			viewQuery = existing.ViewQuery
		}
		viewDDL, vErr := buildCreateViewSQL(existing.Name, viewQuery)
		if vErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, vErr)
		}
		alterStmts = []string{viewDDL}
		cleanedFields = []models.FieldDef{}
		newIndexes = []models.IndexDef{}
	} else {
		tableName := physicalTableName(existing.Name)
		alterStmts, cleanedFields, err = buildAlterTableStatements(tableName, existing.Fields, newFields)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
		}
		if newIndexes == nil {
			newIndexes = existing.Indexes
		}
		if newIndexes == nil {
			newIndexes = []models.IndexDef{}
		}
		indexStmts, err = buildIndexDiffStatements(tableName, existing.Indexes, newIndexes)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
		}
	}

	fieldsJSON, _ := json.Marshal(cleanedFields)
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
			return nil, fmt.Errorf("alter %s: %w", existing.Type, err)
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
		SET view_query = $2, fields = $3, api_rules = $4, indexes = $5, updated_at = NOW()
		WHERE name = $1
		RETURNING id, name, type, view_query, id_type, id_autogenerate_pattern, fields, api_rules, indexes, created_at, updated_at`,
		existing.Name, viewQuery, fieldsJSON, rulesJSON, indexesJSON,
	).Scan(&updated.ID, &updated.Name, &updated.Type, &updated.ViewQuery, &updated.IDType, &updated.IDAutogeneratePattern, &rawFields, &rawRules, &rawIndexes, &updated.CreatedAt, &updated.UpdatedAt)
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

// Truncate removes all records from a collection table without dropping its schema.
func (s *CollectionService) Truncate(ctx context.Context, name string) error {
	col, err := s.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if col.Type == "view" {
		return fmt.Errorf("%w: view collections cannot be truncated", ErrInvalidSchema)
	}
	tableName := physicalTableName(col.Name)
	_, err = s.pool.Exec(ctx, fmt.Sprintf(`TRUNCATE TABLE "%s" RESTART IDENTITY CASCADE`, tableName))
	return err
}

// Duplicate clones an existing collection's schema (fields, api_rules, id_type) under a new name.
func (s *CollectionService) Duplicate(ctx context.Context, sourceName, newName string) (*models.Collection, error) {
	src, err := s.GetByName(ctx, sourceName)
	if err != nil {
		return nil, err
	}
	if newName == "" {
		newName = sourceName + "_copy"
	}
	return s.Create(ctx, models.CreateCollectionRequest{
		Name:                  newName,
		Type:                  src.Type,
		ViewQuery:             src.ViewQuery,
		IDType:                src.IDType,
		IDAutogeneratePattern: src.IDAutogeneratePattern,
		Fields:                src.Fields,
		APIRules:              src.APIRules,
	})
}
