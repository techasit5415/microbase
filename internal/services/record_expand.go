package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

// ExpandRecords resolves PocketBase-style ?expand=field1,rel.subRel,child_via_parent relation fields on a slice of records.
// Supports single-value relations, multi-value JSONB relations, back-relations (<table>_via_<field>), and nested paths (up to 6 levels).
func (s *RecordService) ExpandRecords(ctx context.Context, col *models.Collection, items []models.Record, expandParam string, claims *AuthClaims) {
	s.expandRecordsDepth(ctx, col, items, expandParam, claims, 0)
}

func (s *RecordService) expandRecordsDepth(ctx context.Context, col *models.Collection, items []models.Record, expandParam string, claims *AuthClaims, depth int) {
	if col == nil || len(items) == 0 || strings.TrimSpace(expandParam) == "" || depth >= 6 {
		return
	}

	fieldMap := make(map[string]models.FieldDef, len(col.Fields))
	for _, f := range col.Fields {
		fieldMap[f.Name] = f
	}

	// Group nested sub-expands by top-level key (e.g. "instance.email" -> key="instance", sub="email")
	topOrder := []string{}
	subExpands := map[string][]string{}
	for _, rawName := range strings.Split(expandParam, ",") {
		spec := strings.TrimSpace(rawName)
		if spec == "" {
			continue
		}
		top := spec
		rest := ""
		if dot := strings.IndexByte(spec, '.'); dot > 0 {
			top = spec[:dot]
			rest = spec[dot+1:]
		}
		if _, seen := subExpands[top]; !seen {
			topOrder = append(topOrder, top)
			subExpands[top] = nil
		}
		if rest != "" {
			subExpands[top] = append(subExpands[top], rest)
		}
	}

	for _, fname := range topOrder {
		// Back-relation syntax: <childCollection>_via_<relationField>
		if viaIdx := strings.Index(fname, "_via_"); viaIdx > 0 && viaIdx+5 < len(fname) {
			childName := fname[:viaIdx]
			relField := fname[viaIdx+5:]
			expandedChildren, childCol := s.expandBackRelation(ctx, items, fname, childName, relField, claims)
			if childCol != nil && len(expandedChildren) > 0 && len(subExpands[fname]) > 0 {
				s.expandRecordsDepth(ctx, childCol, expandedChildren, strings.Join(subExpands[fname], ","), claims, depth+1)
			}
			continue
		}

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
		visList := make([]models.Record, 0, len(related))
		for _, rec := range related {
			vis := VisibleRecord(target, claims, rec)
			byID[fmt.Sprint(rec["id"])] = vis
			visList = append(visList, vis)
		}

		if len(subExpands[fname]) > 0 && len(visList) > 0 {
			s.expandRecordsDepth(ctx, target, visList, strings.Join(subExpands[fname], ","), claims, depth+1)
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

func (s *RecordService) expandBackRelation(ctx context.Context, parents []models.Record, expandKey, childName, relField string, claims *AuthClaims) ([]models.Record, *models.Collection) {
	if !models.IsValidIdentifier(childName) || !models.IsValidIdentifier(relField) {
		return nil, nil
	}
	childCol, err := s.collections.GetByName(ctx, childName)
	if err != nil {
		return nil, nil
	}
	var hasField bool
	for _, f := range childCol.Fields {
		if f.Name == relField && f.Type == "relation" {
			hasField = true
			break
		}
	}
	if !hasField {
		return nil, nil
	}
	rule, ruleArgs, _, err := CompileAPIRule(childCol, "view", claims, nil, 2)
	if err != nil {
		return nil, nil
	}

	parentIDs := make([]string, 0, len(parents))
	for _, p := range parents {
		if id := strings.TrimSpace(fmt.Sprint(p["id"])); id != "" {
			parentIDs = append(parentIDs, id)
		}
	}
	if len(parentIDs) == 0 {
		return nil, childCol
	}

	childTable := physicalTableName(childName)
	query := fmt.Sprintf(
		`SELECT * FROM "%s" WHERE ((jsonb_typeof(to_jsonb("%s")) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(to_jsonb("%s")) _el WHERE _el = ANY($1::text[]))) OR CAST("%s" AS TEXT) = ANY($1::text[]))`,
		childTable, relField, relField, relField,
	)
	args := []any{parentIDs}
	if rule != "" {
		query += " AND (" + rule + ")"
		args = append(args, ruleArgs...)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, childCol
	}
	children, err := scanRows(rows)
	rows.Close()
	if err != nil {
		return nil, childCol
	}

	byParent := map[string][]models.Record{}
	allVis := make([]models.Record, 0, len(children))
	for _, ch := range children {
		vis := VisibleRecord(childCol, claims, ch)
		allVis = append(allVis, vis)
		raw := ch[relField]
		switch arr := raw.(type) {
		case []any:
			for _, elem := range arr {
				pid := strings.TrimSpace(fmt.Sprint(elem))
				if pid != "" {
					byParent[pid] = append(byParent[pid], vis)
				}
			}
		default:
			pid := strings.TrimSpace(fmt.Sprint(raw))
			if pid != "" {
				byParent[pid] = append(byParent[pid], vis)
			}
		}
	}

	for _, p := range parents {
		pid := strings.TrimSpace(fmt.Sprint(p["id"]))
		if matched := byParent[pid]; len(matched) > 0 {
			exp, _ := p["expand"].(map[string]any)
			if exp == nil {
				exp = map[string]any{}
				p["expand"] = exp
			}
			exp[expandKey] = matched
		}
	}
	return allVis, childCol
}
