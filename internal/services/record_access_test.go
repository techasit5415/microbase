package services

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
	"os"
	"testing"
	"time"
)

func TestRelationAndVectorAccess(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if os.Getenv("MICROBASE_TEST_LOCAL_ENV") == "1" {
		env, err := godotenv.Read("../../.env")
		if err != nil {
			t.Fatal(err)
		}
		url = env["DATABASE_URL"]
	}
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for integration tests")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid database config")
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer pool.Close()
	_, _ = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE regression_records (id TEXT PRIMARY KEY, owner TEXT, secret TEXT, embedding vector(2)); INSERT INTO regression_records VALUES ('first','1','hidden','[1,0]'),('second','2','hidden','[0,1]')`)
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewMemoryStore()
	defer store.Close()
	col := &models.Collection{Name: "regression_records", IDType: "text", Fields: []models.FieldDef{{Name: "owner", Type: "text"}, {Name: "secret", Type: "text", Hidden: true}, {Name: "embedding", Type: "vector", Dimension: 2}}, APIRules: map[string]string{"view": "owner = @request.auth.id", "list": "owner = @request.auth.id"}}
	raw, _ := json.Marshal(col)
	store.Set(ctx, schemaCacheKey(col.Name), string(raw), time.Minute)
	svc := NewRecordService(pool, NewCollectionService(pool, store), nil, nil)
	source := &models.Collection{Fields: []models.FieldDef{{Name: "related", Type: "relation", CollectionName: col.Name}}}
	items := []models.Record{{"related": "first"}, {"related": "second"}}
	claims := &AuthClaims{UserID: "1", Role: "user"}
	svc.ExpandRecords(ctx, source, items, "related", claims)
	exp, ok := items[0]["expand"].(map[string]any)
	if !ok {
		t.Fatal("text relation not expanded")
	}
	related := exp["related"].(models.Record)
	if _, ok := related["secret"]; ok {
		t.Fatal("hidden field exposed")
	}
	if _, ok := items[1]["expand"]; ok {
		t.Fatal("unauthorized relation exposed")
	}
	result, err := svc.VectorSearch(ctx, col.Name, models.VectorSearchRequest{Field: "embedding", Vector: []float64{1, 0}}, claims)
	if err != nil || len(result) != 1 || result[0]["id"] != "first" {
		t.Fatalf("vector rule failed: %v %v", result, err)
	}
	if !svc.CanViewSnapshot(ctx, col, claims, models.Record{"id": "gone", "owner": "1"}) {
		t.Fatal("owner delete event denied")
	}
	if svc.CanViewSnapshot(ctx, col, claims, models.Record{"id": "gone", "owner": "2"}) {
		t.Fatal("other owner delete event exposed")
	}

}
