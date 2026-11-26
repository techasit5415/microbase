package services

import (
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

// buildWhereClause constructs a parameterized WHERE clause for exact filters and full-text search.
func buildWhereClause(
	col *models.Collection,
	opts models.ListOptions,
	allowed map[string]models.FieldDef,
) (string, []any, int, error) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	idx := 1

	// Enforce collection "list" API rule (including row-level SQL expressions).
	claims, _ := opts.AuthClaims.(*AuthClaims)
	ruleSQL, ruleArgs, nextRuleIdx, ruleErr := CompileAPIRule(col, "list", claims, nil, idx)
	if ruleErr != nil {
		return "", nil, 0, ruleErr
	}
	if ruleSQL != "" {
		clauses = append(clauses, "("+ruleSQL+")")
		args = append(args, ruleArgs...)
		idx = nextRuleIdx
	}

	// Exact-match filters (whitelisted against collection schema).
	for key, val := range opts.Filters {
		if _, ok := allowed[key]; !ok && !reservedColumns[key] {
			return "", nil, idx, fmt.Errorf("%w: unknown filter field %q", ErrInvalidInput, key)
		}
		clauses = append(clauses, fmt.Sprintf(`"%s"::text = $%d`, key, idx))
		args = append(args, val)
		idx++
	}

	// Full-text + substring search across id, auth email, and all text-like fields (supports English FTS and Thai).
	if q := strings.TrimSpace(opts.Search); q != "" {
		textCols := []string{`coalesce(CAST("id" AS TEXT), '')`}
		seen := map[string]bool{"id": true}
		if col.Name == "users" || col.Type == "auth" {
			textCols = append(textCols, `coalesce("email", '')`)
			seen["email"] = true
		}
		for _, f := range col.Fields {
			if seen[f.Name] || f.Hidden || f.Type == "vector" || f.Type == "password" {
				continue
			}
			textCols = append(textCols, fmt.Sprintf(`coalesce(CAST("%s" AS TEXT), '')`, f.Name))
			seen[f.Name] = true
		}
		if len(textCols) > 0 {
			concatExpr := fmt.Sprintf("concat_ws(' ', %s)", strings.Join(textCols, ", "))
			clauses = append(clauses, fmt.Sprintf(
				`(to_tsvector('simple', %s) @@ websearch_to_tsquery('simple', $%d) OR %s ILIKE $%d)`,
				concatExpr, idx, concatExpr, idx+1,
			))
			args = append(args, q, "%"+q+"%")
			idx += 2
		}
	}

	if strings.TrimSpace(opts.FilterExpr) != "" {
		fSQL, fArgs, nextIdx, err := ParseFilterToSQL(opts.FilterExpr, allowed, claims, idx)
		if err != nil {
			return "", nil, 0, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		clauses = append(clauses, fSQL)
		args = append(args, fArgs...)
		idx = nextIdx
	}

	if len(clauses) == 0 {
		return "", args, idx, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, idx, nil
}

// buildOrderBy converts a sort expression (e.g. "-created_at", "title") into a safe SQL ORDER BY clause.
func buildOrderBy(sort string, allowed map[string]models.FieldDef) (string, error) {
	sort = strings.TrimSpace(sort)
	if sort == "" {
		return `"id" DESC`, nil
	}

	parts := strings.Split(sort, ",")
	clauses := make([]string, 0, len(parts))
	for _, raw := range parts {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if strings.EqualFold(item, "@random") {
			clauses = append(clauses, "RANDOM()")
			continue
		}
		dir := "ASC"
		field := item
		if strings.HasPrefix(item, "-") {
			dir = "DESC"
			field = strings.TrimSpace(strings.TrimPrefix(item, "-"))
		} else if strings.HasPrefix(item, "+") {
			field = strings.TrimSpace(strings.TrimPrefix(item, "+"))
		}
		if _, ok := allowed[field]; !ok && !reservedColumns[field] {
			// PocketBase compatibility aliases for collections without explicit created/updated columns
			if field == "created" {
				field = "created_at"
			} else if field == "updated" {
				field = "updated_at"
			} else {
				return "", fmt.Errorf("unknown sort field %q", field)
			}
		}
		clauses = append(clauses, fmt.Sprintf(`"%s" %s`, field, dir))
	}
	if len(clauses) == 0 {
		return `"id" DESC`, nil
	}
	return strings.Join(clauses, ", "), nil
}
