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
	return buildCreateTableSQLWithTargets(req, nil)
}

func buildCreateTableSQLWithTargets(req models.CreateCollectionRequest, targetIDTypes map[string]string) (string, error) {
	if !models.IsValidIdentifier(req.Name) {
		return "", fmt.Errorf("invalid collection name %q", req.Name)
	}

	idCol := `"id" BIGSERIAL PRIMARY KEY`
	if req.IDType == "text" {
		idCol = `"id" TEXT PRIMARY KEY`
	}
	columns := []string{idCol}
	if req.Type == "auth" {
		columns = append(columns,
			`"email" TEXT UNIQUE NOT NULL`,
			`"password_hash" TEXT NOT NULL DEFAULT ''`,
			`"token_key" TEXT UNIQUE NOT NULL DEFAULT ''`,
			`"email_visibility" BOOLEAN NOT NULL DEFAULT FALSE`,
			`"verified" BOOLEAN NOT NULL DEFAULT FALSE`,
		)
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

		pgType, err := resolveFieldPostgresType(f, targetIDTypes)
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
		if f.Type == "select" && len(f.Values) == 0 {
			return "", fmt.Errorf("select field %q must define at least one option value", f.Name)
		}
		if fkSQL, fkErr := relationConstraintSQL(f); fkErr != nil {
			return "", fkErr
		} else if fkSQL != "" {
			colDef += fkSQL
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
	return buildAlterTableStatementsWithTargets(tableName, oldFields, newFields, nil)
}

func buildAlterTableStatementsWithTargets(tableName string, oldFields, newFields []models.FieldDef, targetIDTypes map[string]string) ([]string, []models.FieldDef, error) {
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
			pgType, err := resolveFieldPostgresType(f, targetIDTypes)
			if err != nil {
				return nil, nil, err
			}
			colDef := fmt.Sprintf(`"%s" %s`, f.Name, pgType)
			if f.Unique {
				colDef += " UNIQUE"
			}
			if f.Type == "select" && len(f.Values) == 0 {
				return nil, nil, fmt.Errorf("select field %q must define at least one option value", f.Name)
			}
			if fkSQL, fkErr := relationConstraintSQL(f); fkErr != nil {
				return nil, nil, fkErr
			} else if fkSQL != "" {
				colDef += fkSQL
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

// buildCreateIndexSQL validates an IndexDef and generates a PostgreSQL CREATE [UNIQUE] INDEX statement.
func buildCreateIndexSQL(tableName string, idx models.IndexDef) (string, error) {
	if tableName != "_users" && !models.IsValidIdentifier(tableName) {
		return "", fmt.Errorf("invalid table name %q", tableName)
	}
	if !models.IsValidIdentifier(idx.Name) {
		return "", fmt.Errorf("invalid index name %q", idx.Name)
	}
	if len(idx.Columns) == 0 {
		return "", fmt.Errorf("index %q must include at least one column", idx.Name)
	}
	quotedCols := make([]string, 0, len(idx.Columns))
	for _, col := range idx.Columns {
		if !models.IsValidIdentifier(col) {
			return "", fmt.Errorf("invalid column %q in index %q", col, idx.Name)
		}
		quotedCols = append(quotedCols, fmt.Sprintf(`"%s"`, col))
	}
	uniqueKW := ""
	if idx.Unique {
		uniqueKW = "UNIQUE "
	}
	return fmt.Sprintf(`CREATE %sINDEX IF NOT EXISTS "%s" ON "%s" (%s);`,
		uniqueKW, idx.Name, tableName, strings.Join(quotedCols, ", ")), nil
}

// buildDropIndexSQL generates a safe PostgreSQL DROP INDEX statement.
func buildDropIndexSQL(idxName string) (string, error) {
	if !models.IsValidIdentifier(idxName) {
		return "", fmt.Errorf("invalid index name %q", idxName)
	}
	return fmt.Sprintf(`DROP INDEX IF EXISTS "%s";`, idxName), nil
}

func indexSignature(idx models.IndexDef) string {
	return fmt.Sprintf("%s|%t|%s", idx.Name, idx.Unique, strings.Join(idx.Columns, ","))
}

// buildIndexDiffStatements computes DROP/CREATE INDEX DDL statements between old and new IndexDef slices.
func buildIndexDiffStatements(tableName string, oldIndexes, newIndexes []models.IndexDef) ([]string, error) {
	oldMap := make(map[string]models.IndexDef, len(oldIndexes))
	for _, idx := range oldIndexes {
		oldMap[idx.Name] = idx
	}
	newMap := make(map[string]models.IndexDef, len(newIndexes))
	for _, idx := range newIndexes {
		newMap[idx.Name] = idx
	}

	var stmts []string

	// Drop removed or modified indexes first.
	for name, oldIdx := range oldMap {
		newIdx, exists := newMap[name]
		if !exists || indexSignature(oldIdx) != indexSignature(newIdx) {
			dropSQL, err := buildDropIndexSQL(name)
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, dropSQL)
		}
	}

	// Create newly added or modified indexes.
	for _, newIdx := range newIndexes {
		oldIdx, exists := oldMap[newIdx.Name]
		if !exists || indexSignature(oldIdx) != indexSignature(newIdx) {
			createSQL, err := buildCreateIndexSQL(tableName, newIdx)
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, createSQL)
		}
	}

	return stmts, nil
}

// relationConstraintSQL builds a PostgreSQL REFERENCES clause for single-value relation fields.
func relationConstraintSQL(f models.FieldDef) (string, error) {
	if f.Type != "relation" {
		return "", nil
	}
	if f.CollectionName == "" {
		return "", fmt.Errorf("relation field %q must specify a target collection_name", f.Name)
	}
	targetTable := f.CollectionName
	if targetTable == "users" {
		targetTable = "_users"
	} else if !models.IsValidIdentifier(targetTable) {
		return "", fmt.Errorf("invalid target collection %q for relation field %q", f.CollectionName, f.Name)
	}
	if f.MaxSelect > 1 {
		return "", nil // Multi-relation stored as JSONB array of IDs
	}
	onDelete := "ON DELETE SET NULL"
	if f.CascadeDelete {
		onDelete = "ON DELETE CASCADE"
	}
	return fmt.Sprintf(` REFERENCES "%s"(id) %s`, targetTable, onDelete), nil
}

// buildCreateViewSQL validates a read-only SELECT/WITH query and builds CREATE OR REPLACE VIEW.
func buildCreateViewSQL(name, viewQuery string) (string, error) {
	if !models.IsValidIdentifier(name) {
		return "", fmt.Errorf("invalid view collection name %q", name)
	}
	q := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(viewQuery), ";"))
	if q == "" {
		return "", fmt.Errorf("view collection %q requires a SELECT query", name)
	}
	if strings.Contains(q, ";") {
		return "", fmt.Errorf("view query must be a single SELECT statement without semicolons")
	}
	upper := strings.ToUpper(q)
	if !strings.HasPrefix(upper, "SELECT ") && !strings.HasPrefix(upper, "WITH ") {
		return "", fmt.Errorf("view query must start with SELECT or WITH")
	}
	return fmt.Sprintf(`CREATE OR REPLACE VIEW "%s" AS %s;`, name, q), nil
}

// resolveFieldPostgresType resolves a field's PostgreSQL column type, matching TEXT vs BIGINT for single relations.
func resolveFieldPostgresType(f models.FieldDef, targetIDTypes map[string]string) (string, error) {
	if f.Type == "relation" && f.MaxSelect <= 1 {
		if targetIDTypes != nil && targetIDTypes[f.CollectionName] == "text" {
			return "TEXT", nil
		}
		return "BIGINT", nil
	}
	return f.ToPostgresType()
}
