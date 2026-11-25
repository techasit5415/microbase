package services

import (
	"context"
	"fmt"

	"github.com/techasit5415/microbase/internal/models"
)

// VectorSearch ranks records by cosine distance (<=>) against a pgvector column.
func (s *RecordService) VectorSearch(ctx context.Context, colName string, req models.VectorSearchRequest) ([]models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}

	allowed := allowedFieldMap(col)
	field, ok := allowed[req.Field]
	if !ok || field.Type != "vector" {
		return nil, fmt.Errorf("%w: %q is not a vector field", ErrInvalidInput, req.Field)
	}
	if len(req.Vector) != field.Dimension {
		return nil, fmt.Errorf("%w: vector expects %d dimensions, got %d", ErrInvalidInput, field.Dimension, len(req.Vector))
	}

	limit := req.Limit
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	vecLiteral := formatVectorLiteral(req.Vector)
	querySQL := fmt.Sprintf(
		`SELECT *, ("%s" <=> $1::vector) AS "_distance" FROM "%s" WHERE "%s" IS NOT NULL ORDER BY "%s" <=> $1::vector ASC LIMIT $2`,
		field.Name, col.Name, field.Name, field.Name,
	)

	rows, err := s.pool.Query(ctx, querySQL, vecLiteral, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	return scanRows(rows)
}
