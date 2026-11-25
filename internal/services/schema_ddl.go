package services

import (
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

// reservedColumns are auto-managed by Microbase on every collection table.
var reservedColumns = map[string]bool{
	"id":         true,
	"created_at": true,
	"updated_at": true,
}

// buildCreateTableSQL validates identifiers and constructs a safe PostgreSQL DDL statement.
func buildCreateTableSQL(req models.CreateCollectionRequest) (string, error) {
	if !models.IsValidIdentifier(req.Name) {
		return "", fmt.Errorf("invalid collection name %q", req.Name)
	}

	columns := []string{
		`"id" BIGSERIAL PRIMARY KEY`,
	}

	seen := make(map[string]bool)
	for _, f := range req.Fields {
		if !models.IsValidIdentifier(f.Name) {
			return "", fmt.Errorf("invalid field name %q", f.Name)
		}
		if reservedColumns[f.Name] {
			return "", fmt.Errorf("field name %q is reserved", f.Name)
		}
		if seen[f.Name] {
			return "", fmt.Errorf("duplicate field name %q", f.Name)
		}
		seen[f.Name] = true

		pgType, err := f.ToPostgresType()
		if err != nil {
			return "", err
		}

		colDef := fmt.Sprintf(`"%s" %s`, f.Name, pgType)
		if f.Required {
			colDef += " NOT NULL"
		}
		if f.Unique {
			colDef += " UNIQUE"
		}
		columns = append(columns, colDef)
	}

	columns = append(columns,
		`"created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`"updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
	)

	return fmt.Sprintf(`CREATE TABLE "%s" (%s);`, req.Name, strings.Join(columns, ", ")), nil
}
