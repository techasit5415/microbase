package services

import (
	"os"
	"strings"
	"testing"
)

func TestParsePocketBaseSchemaAndDDL(t *testing.T) {
	sample := `[
		{"id":"pbc_sys","name":"_superusers","type":"auth","system":true,"fields":[]},
		{
			"id":"pbc_logs",
			"name":"Logs",
			"type":"base",
			"listRule":"",
			"viewRule":null,
			"fields":[
				{"name":"id","type":"text","system":true,"autogeneratePattern":"[a-z0-9]{15}"},
				{"name":"user_id","type":"relation","collectionId":"_pb_users_auth_","maxSelect":1},
				{"name":"category","type":"relation","collectionId":"pbc_cat","maxSelect":1},
				{"name":"bookerName","type":"text","required":true},
				{"name":"active","type":"bool"},
				{"name":"dueDate","type":"date"},
				{"name":"created","type":"autodate","onCreate":true}
			],
			"indexes":[
				"CREATE INDEX idx_logs_cat ON Logs (category, created DESC)"
			]
		},
		{
			"id":"pbc_cat",
			"name":"Categories",
			"type":"base",
			"fields":[
				{"name":"id","type":"text","system":true,"autogeneratePattern":"[a-z0-9]{15}"},
				{"name":"title","type":"text","required":true}
			]
		}
	]`

	export, err := ParsePocketBaseSchema([]byte(sample))
	if err != nil {
		t.Fatalf("ParsePocketBaseSchema failed: %v", err)
	}
	if len(export.Collections) != 2 {
		t.Fatalf("expected 2 non-system collections, got %d", len(export.Collections))
	}

	ordered := orderCollectionsByRelationDeps(export.Collections)
	if ordered[0].Name != "Categories" || ordered[1].Name != "Logs" {
		t.Fatalf("expected Categories before Logs, got %s then %s", ordered[0].Name, ordered[1].Name)
	}

	// Verify DDL uses TEXT FK for text-ID target (Categories) and BIGINT FK for serial-ID target (users/_users).
	targetIDTypes := map[string]string{
		"users":      "serial",
		"Categories": "text",
	}
	ddl, err := buildCreateTableSQLWithTargets(ordered[1], targetIDTypes)
	if err != nil {
		t.Fatalf("buildCreateTableSQLWithTargets(Logs) failed: %v", err)
	}
	if !strings.Contains(ddl, `"id" TEXT PRIMARY KEY`) {
		t.Fatalf("expected TEXT primary key in DDL: %s", ddl)
	}
	if !strings.Contains(ddl, `"user_id" BIGINT REFERENCES "_users"(id)`) {
		t.Fatalf("expected BIGINT FK to _users in DDL: %s", ddl)
	}
	if !strings.Contains(ddl, `"category" TEXT REFERENCES "Categories"(id)`) {
		t.Fatalf("expected TEXT FK to Categories in DDL: %s", ddl)
	}
	if !strings.Contains(ddl, `"bookerName" TEXT NOT NULL`) {
		t.Fatalf("expected preserved camelCase bookerName in DDL: %s", ddl)
	}
	if len(ordered[1].Indexes) != 1 || ordered[1].Indexes[0].Name != "idx_logs_cat" {
		t.Fatalf("expected parsed index idx_logs_cat, got %+v", ordered[1].Indexes)
	}
}

func TestParseRealPBSchemaIfPresent(t *testing.T) {
	raw, err := os.ReadFile("/mnt/c/Users/techa/Downloads/pb_schema.json")
	if err != nil {
		t.Skip("local pb_schema.json not present")
	}
	export, err := ParsePocketBaseSchema(raw)
	if err != nil {
		t.Fatalf("ParsePocketBaseSchema(pb_schema.json) failed: %v", err)
	}
	ordered := orderCollectionsByRelationDeps(export.Collections)
	targetIDTypes := map[string]string{"users": "serial"}
	for _, c := range ordered {
		if c.Name == "users" {
			continue
		}
		ddl, err := buildCreateTableSQLWithTargets(c, targetIDTypes)
		if err != nil {
			t.Fatalf("DDL failed for %q: %v", c.Name, err)
		}
		if ddl == "" {
			t.Fatalf("empty DDL for %q", c.Name)
		}
		for _, idx := range c.Indexes {
			if _, err := buildCreateIndexSQL(c.Name, idx); err != nil {
				t.Fatalf("index DDL failed for %q.%q: %v", c.Name, idx.Name, err)
			}
		}
		targetIDTypes[c.Name] = c.IDType
	}
	t.Logf("Validated all %d collections from pb_schema.json cleanly!", len(ordered))
}
