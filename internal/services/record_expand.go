package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

// ExpandRecords resolves PocketBase-style ?expand=field1,field2 relation fields on a slice of records.
// Supports both single-value relations (expanded to object) and multi-value JSONB relations (expanded to array).
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
		if !models.IsValidIdentifier(targetTable) {
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

		idSet := map[string]bool{}
		ids := []string{}
		addID := func(v any) {
			if v == nil {
				return
			}
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && !idSet[s] {
				idSet[s] = true
				ids = append(ids, s)
			}
		}

		for _, rec := range items {
			raw := rec[fname]
			if raw == nil {
				continue
			}
			switch arr := raw.(type) {
			case []any:
				for _, elem := range arr {
					addID(elem)
				}
			case []string:
				for _, elem := range arr {
					addID(elem)
				}
			default:
				addID(raw)
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
			raw := rec[fname]
			if raw == nil {
				continue
			}
			switch arr := raw.(type) {
			case []any:
				matched := make([]models.Record, 0, len(arr))
				for _, elem := range arr {
					if rel, ok := byID[fmt.Sprint(elem)]; ok {
						matched = append(matched, rel)
					}
				}
				if len(matched) > 0 {
					exp, _ := rec["expand"].(map[string]any)
					if exp == nil {
						exp = map[string]any{}
						rec["expand"] = exp
					}
					exp[fname] = matched
				}
			default:
				if rel, ok := byID[fmt.Sprint(raw)]; ok {
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
}
