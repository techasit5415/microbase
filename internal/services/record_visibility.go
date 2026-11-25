package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/techasit5415/microbase/internal/models"
)

func VisibleRecord(col *models.Collection, claims *AuthClaims, rec models.Record) models.Record {
	out := make(models.Record, len(rec))
	for k, v := range rec {
		if k != "password_hash" {
			out[k] = v
		}
	}
	if claims == nil || claims.Role != "superuser" {
		for _, f := range col.Fields {
			if f.Hidden {
				delete(out, f.Name)
			}
		}
	}
	return out
}

// CanViewSnapshot uses the same SQL rule semantics as normal record reads.
func (s *RecordService) CanViewSnapshot(ctx context.Context, col *models.Collection, claims *AuthClaims, snapshot any) bool {
	rule, args, _, err := CompileAPIRule(col, "view", claims, nil, 2)
	if err != nil {
		return false
	}
	if rule == "" {
		return true
	}
	if snapshot == nil {
		return false
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return false
	}
	params := append([]any{string(data)}, args...)
	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM json_populate_record(NULL::"%s", $1::json) WHERE (%s))`, physicalTableName(col.Name), rule)
	var allowed bool
	return s.pool.QueryRow(ctx, query, params...).Scan(&allowed) == nil && allowed
}
