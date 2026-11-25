package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

// ExpandRecords resolves PocketBase-style ?expand=field1,field2 relation fields on a slice of records.
func (s *RecordService) ExpandRecords(ctx context.Context, col *models.Collection, items []models.Record, expandParam string, claims *AuthClaims) {
	if col == nil || len(items) == 0 || strings.TrimSpace(expandParam) == "" {
		return
	}

	fieldMap := make(map[string]models.FieldDef, len(col.Fields))
	for _, f := range col.Fields {
		fieldMap[f.Name] = f
	}

	reqFields := strings.Split(expandParam, ",")
	for _, rawName := range reqFields {
		fname := strings.TrimSpace(rawName)
		f, ok := fieldMap[fname]
		if !ok || f.Type != "relation" || f.CollectionName == "" {
			continue
		}

		targetTable := physicalTableName(f.CollectionName)
		if targetTable != "_users" && !models.IsValidIdentifier(targetTable) {
			continue
		}

		target, err := s.collections.GetByName(ctx, f.CollectionName)
		if err != nil {
			continue
		}
		if f.Hidden && (claims == nil || claims.Role != "superuser") {
			continue
		}
		rule, ruleArgs, _, err := CompileAPIRule(target, "view", claims, nil, 2)
		if err != nil {
			continue
		}
		ids := []string{}
		for _, rec := range items {
			if rec[fname] != nil {
				ids = append(ids, fmt.Sprint(rec[fname]))
			}
		}
		if len(ids) == 0 {
			continue
		}
		query := fmt.Sprintf(`SELECT * FROM "%s" WHERE id::text = ANY($1::text[])`, targetTable)
		args := []any{ids}
		if rule != "" {
			query += " AND (" + rule + ")"
			args = append(args, ruleArgs...)
		}
		rows, err := s.pool.Query(ctx, query, args...)
		if err != nil {
			continue
		}
		related, err := scanRows(rows)
		rows.Close()
		if err != nil {
			continue
		}
		byID := map[string]models.Record{}
		for _, rec := range related {
			byID[fmt.Sprint(rec["id"])] = VisibleRecord(target, claims, rec)
		}
		for _, rec := range items {
			if rel, ok := byID[fmt.Sprint(rec[fname])]; ok {
				exp, _ := rec["expand"].(map[string]any)
				if exp == nil {
					exp = map[string]any{}
					rec["expand"] = exp
				}
				exp[fname] = rel
			}
		}
	}
}
