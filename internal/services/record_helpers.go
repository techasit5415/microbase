package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/techasit5415/microbase/internal/models"
)

// scanRows reads dynamic pgx.Rows into a slice of models.Record keyed by column name.
func scanRows(rows pgx.Rows) ([]models.Record, error) {
	fieldDescs := rows.FieldDescriptions()
	items := make([]models.Record, 0)

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read row values: %w", err)
		}

		rec := make(models.Record, len(fieldDescs))
		for i, fd := range fieldDescs {
			rec[fd.Name] = values[i]
		}
		items = append(items, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// allowedFieldMap indexes a collection's user-defined fields by name for O(1) validation.
func allowedFieldMap(col *models.Collection) map[string]models.FieldDef {
	allowed := make(map[string]models.FieldDef, len(col.Fields)+4)
	if col.Type == "auth" || col.Name == "users" {
		allowed["email"] = models.FieldDef{Name: "email", Type: "email"}
		allowed["role"] = models.FieldDef{Name: "role", Type: "text"}
		allowed["verified"] = models.FieldDef{Name: "verified", Type: "boolean"}
		allowed["email_visibility"] = models.FieldDef{Name: "email_visibility", Type: "boolean"}
	}
	for _, f := range col.Fields {
		allowed[f.Name] = f
	}
	return allowed
}

// formatVectorLiteral encodes a float slice into pgvector's text format, e.g. "[0.1,0.2,0.3]".
func formatVectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// coerceInputValue prepares an incoming JSON value for its target PostgreSQL column type.
func coerceInputValue(field models.FieldDef, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}

	switch field.Type {
	case "vector":
		arr, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("field %q must be an array of numbers", field.Name)
		}
		if len(arr) != field.Dimension {
			return nil, fmt.Errorf("field %q expects %d dimensions, got %d", field.Name, field.Dimension, len(arr))
		}
		vec := make([]float64, len(arr))
		for i, item := range arr {
			num, ok := item.(float64)
			if !ok {
				return nil, fmt.Errorf("field %q element %d must be a number", field.Name, i)
			}
			vec[i] = num
		}
		return formatVectorLiteral(vec), nil

	case "json", "geopoint":
		encoded, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("field %q invalid json: %w", field.Name, err)
		}
		return encoded, nil

	default:
		return raw, nil
	}
}
