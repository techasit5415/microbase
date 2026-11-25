package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

// ExpandRecords resolves PocketBase-style ?expand=field1,field2 relation fields on a slice of records.
func (s *RecordService) ExpandRecords(ctx context.Context, col *models.Collection, items []models.Record, expandParam string) {
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

		// Collect unique integer IDs referenced across items.
		idSet := make(map[int64]struct{})
		for _, rec := range items {
			switch v := rec[fname].(type) {
			case int64:
				idSet[v] = struct{}{}
			case int32:
				idSet[int64(v)] = struct{}{}
			case float64:
				idSet[int64(v)] = struct{}{}
			}
		}
		if len(idSet) == 0 {
			continue
		}

		ids := make([]int64, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}

		query := fmt.Sprintf(`SELECT * FROM "%s" WHERE id = ANY($1)`, targetTable)
		rows, err := s.pool.Query(ctx, query, ids)
		if err != nil {
			continue
		}
		descs := rows.FieldDescriptions()
		byID := make(map[int64]models.Record, len(ids))
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				continue
			}
			rel := make(models.Record, len(descs))
			var rid int64
			for i, fd := range descs {
				colName := string(fd.Name)
				if colName == "password_hash" {
					continue
				}
				rel[colName] = vals[i]
				if colName == "id" {
					switch tv := vals[i].(type) {
					case int64:
						rid = tv
					case int32:
						rid = int64(tv)
					}
				}
			}
			if rid != 0 {
				byID[rid] = rel
			}
		}
		rows.Close()

		for _, rec := range items {
			var rid int64
			switch v := rec[fname].(type) {
			case int64:
				rid = v
			case int32:
				rid = int64(v)
			case float64:
				rid = int64(v)
			}
			if rel, found := byID[rid]; found {
				exp, _ := rec["expand"].(map[string]any)
				if exp == nil {
					exp = make(map[string]any)
					rec["expand"] = exp
				}
				exp[fname] = rel
			}
		}
	}
}
