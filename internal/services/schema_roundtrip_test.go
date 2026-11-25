package services

import (
	"context"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/testutil"
	"testing"
)

func TestSchemaExportPreservesConfiguration(t *testing.T) {
	pool := testutil.Database(t)
	s := NewCollectionService(pool, cache.NewMemoryStore())
	ctx := context.Background()
	req := models.CreateCollectionRequest{Name: "exported", IDType: "text", IDAutogeneratePattern: "[a-z]{12}", Fields: []models.FieldDef{{Name: "title", Type: "text"}}, APIRules: map[string]string{"list": "auth"}, Indexes: []models.IndexDef{{Name: "idx_exported", Columns: []string{"title"}, Unique: true}}}
	if _, err := s.Create(ctx, req); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.ExportSchema(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range snapshot.Collections {
		if col.Name == req.Name {
			if col.IDType != req.IDType || col.IDAutogeneratePattern != req.IDAutogeneratePattern || col.APIRules["list"] != "auth" || len(col.Indexes) != 1 {
				t.Fatalf("export lost schema configuration: %+v", col)
			}
			return
		}
	}
	t.Fatal("collection missing")
}
