package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/techasit5415/microbase/internal/models"
)

// ExportSchema returns a portable JSON snapshot of all collection schemas.
func (s *CollectionService) ExportSchema(ctx context.Context) (*models.SchemaExport, error) {
	cols, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	exported := make([]models.CreateCollectionRequest, 0, len(cols))
	for _, c := range cols {
		exported = append(exported, models.CreateCollectionRequest{
			Name:   c.Name,
			Fields: c.Fields,
		})
	}

	return &models.SchemaExport{
		Version:     "1.0",
		ExportedAt:  time.Now().UTC(),
		Collections: exported,
	}, nil
}

// ImportSchema creates any missing collections from a SchemaExport snapshot (idempotent).
func (s *CollectionService) ImportSchema(ctx context.Context, payload models.SchemaExport) (created []string, skipped []string, err error) {
	created = make([]string, 0)
	skipped = make([]string, 0)

	for _, colReq := range payload.Collections {
		_, createErr := s.Create(ctx, colReq)
		if errors.Is(createErr, ErrCollectionExists) {
			skipped = append(skipped, colReq.Name)
			continue
		}
		if createErr != nil {
			return created, skipped, fmt.Errorf("import %q: %w", colReq.Name, createErr)
		}
		created = append(created, colReq.Name)
	}
	return created, skipped, nil
}
