package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/models"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrInvalidInput   = errors.New("invalid record input")
)

// RecordService provides dynamic CRUD and search operations for registered collections.
type RecordService struct {
	pool        *pgxpool.Pool
	collections *CollectionService
}

// NewRecordService constructs a RecordService.
func NewRecordService(pool *pgxpool.Pool, colSvc *CollectionService) *RecordService {
	return &RecordService{pool: pool, collections: colSvc}
}

// List queries records with pagination, sorting, exact filters, and full-text search.
func (s *RecordService) List(ctx context.Context, colName string, opts models.ListOptions) (*models.ListResult, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}
	opts.Normalize()
	allowed := allowedFieldMap(col)

	whereSQL, args, nextIdx, err := buildWhereClause(col, opts, allowed)
	if err != nil {
		return nil, err
	}

	var total int64
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"%s`, col.Name, whereSQL)
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count records: %w", err)
	}

	orderSQL, err := buildOrderBy(opts.Sort, allowed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	selectSQL := fmt.Sprintf(`SELECT * FROM "%s"%s ORDER BY %s LIMIT $%d OFFSET $%d`,
		col.Name, whereSQL, orderSQL, nextIdx, nextIdx+1)
	args = append(args, opts.PerPage, opts.Offset())

	rows, err := s.pool.Query(ctx, selectSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query records: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}

	return &models.ListResult{
		Page:       opts.Page,
		PerPage:    opts.PerPage,
		TotalItems: total,
		Items:      items,
	}, nil
}

// GetByID fetches a single record by its numeric ID.
func (s *RecordService) GetByID(ctx context.Context, colName string, id int64) (models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT * FROM "%s" WHERE "id" = $1`, col.Name), id)
	if err != nil {
		return nil, fmt.Errorf("get record: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrRecordNotFound
	}
	return items[0], nil
}

// Create validates input against the collection schema and inserts a new record.
func (s *RecordService) Create(ctx context.Context, colName string, data models.Record) (models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}
	allowed := allowedFieldMap(col)

	for _, f := range col.Fields {
		if f.Required {
			if val, exists := data[f.Name]; !exists || val == nil {
				return nil, fmt.Errorf("%w: field %q is required", ErrInvalidInput, f.Name)
			}
		}
	}

	cols, placeholders, args := make([]string, 0, len(data)), make([]string, 0, len(data)), make([]any, 0, len(data))
	idx := 1
	for key, raw := range data {
		field, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalidInput, key)
		}
		val, err := coerceInputValue(field, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		cols = append(cols, fmt.Sprintf(`"%s"`, key))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, val)
		idx++
	}

	insertSQL := fmt.Sprintf(`INSERT INTO "%s" DEFAULT VALUES RETURNING *`, col.Name)
	if len(cols) > 0 {
		insertSQL = fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s) RETURNING *`,
			col.Name, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	}

	rows, err := s.pool.Query(ctx, insertSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("insert record: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// Update modifies specified fields on an existing record and updates updated_at.
func (s *RecordService) Update(ctx context.Context, colName string, id int64, data models.Record) (models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}
	allowed := allowedFieldMap(col)

	setClauses, args := make([]string, 0, len(data)+1), make([]any, 0, len(data)+1)
	idx := 1
	for key, raw := range data {
		field, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalidInput, key)
		}
		val, err := coerceInputValue(field, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		setClauses = append(setClauses, fmt.Sprintf(`"%s" = $%d`, key, idx))
		args = append(args, val)
		idx++
	}
	if len(setClauses) == 0 {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}

	setClauses = append(setClauses, `"updated_at" = NOW()`)
	args = append(args, id)

	updateSQL := fmt.Sprintf(`UPDATE "%s" SET %s WHERE "id" = $%d RETURNING *`,
		col.Name, strings.Join(setClauses, ", "), idx)

	rows, err := s.pool.Query(ctx, updateSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("update record: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrRecordNotFound
	}
	return items[0], nil
}

// Delete removes a record by ID.
func (s *RecordService) Delete(ctx context.Context, colName string, id int64) error {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM "%s" WHERE "id" = $1`, col.Name), id)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}
