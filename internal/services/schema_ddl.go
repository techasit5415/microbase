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

	columns := []string{`"id" BIGSERIAL PRIMARY KEY`}
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

// buildAlterTableStatements computes ADD, DROP, RENAME, and NOT NULL DDL statements safely.
func buildAlterTableStatements(tableName string, oldFields, newFields []models.FieldDef) ([]string, []models.FieldDef, error) {
	oldMap := make(map[string]models.FieldDef, len(oldFields))
	for _, f := range oldFields {
		oldMap[f.Name] = f
	}

	keptOld := make(map[string]bool)
	seen := make(map[string]bool)
	stmts := make([]string, 0)
	cleaned := make([]models.FieldDef, 0, len(newFields))

	for _, f := range newFields {
		if !models.IsValidIdentifier(f.Name) {
			return nil, nil, fmt.Errorf("invalid field name %q", f.Name)
		}
		if reservedColumns[f.Name] {
			return nil, nil, fmt.Errorf("field name %q is reserved", f.Name)
		}
		if seen[f.Name] {
			return nil, nil, fmt.Errorf("duplicate field name %q", f.Name)
		}
		seen[f.Name] = true

		lookupName := f.OriginalName
		if lookupName == "" {
			lookupName = f.Name
		}

		if oldF, existed := oldMap[lookupName]; existed {
			keptOld[oldF.Name] = true

			// Rename column if name changed.
			if oldF.Name != f.Name {
				stmts = append(stmts, fmt.Sprintf(`ALTER TABLE "%s" RENAME COLUMN "%s" TO "%s";`, tableName, oldF.Name, f.Name))
			}
			// Toggle NOT NULL if required flag changed.
			if oldF.Required != f.Required {
				if f.Required {
					stmts = append(stmts, fmt.Sprintf(`ALTER TABLE "%s" ALTER COLUMN "%s" SET NOT NULL;`, tableName, f.Name))
				} else {
					stmts = append(stmts, fmt.Sprintf(`ALTER TABLE "%s" ALTER COLUMN "%s" DROP NOT NULL;`, tableName, f.Name))
				}
			}
		} else {
			pgType, err := f.ToPostgresType()
			if err != nil {
				return nil, nil, err
			}
			colDef := fmt.Sprintf(`"%s" %s`, f.Name, pgType)
			if f.Unique {
				colDef += " UNIQUE"
			}
			stmts = append(stmts, fmt.Sprintf(`ALTER TABLE "%s" ADD COLUMN %s;`, tableName, colDef))
		}

		// Strip transient OriginalName before persisting metadata.
		f.OriginalName = ""
		cleaned = append(cleaned, f)
	}

	for _, oldF := range oldFields {
		if !keptOld[oldF.Name] {
			stmts = append(stmts, fmt.Sprintf(`ALTER TABLE "%s" DROP COLUMN IF EXISTS "%s";`, tableName, oldF.Name))
		}
	}

	return stmts, cleaned, nil
}
