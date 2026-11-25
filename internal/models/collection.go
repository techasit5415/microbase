package models

import (
	"fmt"
	"regexp"
	"time"
)

// identifierRegex enforces PostgreSQL-safe lowercase identifiers up to 63 bytes.
// Leading underscores are disallowed to reserve system tables (e.g., _collections, _users).
var identifierRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// FieldDef defines the schema for a single column within a collection.
type FieldDef struct {
	Name      string `json:"name"`
	Type      string `json:"type"`                // Supported: text, integer, float, boolean, json, timestamptz, vector
	Required  bool   `json:"required,omitempty"`  // Maps to NOT NULL
	Unique    bool   `json:"unique,omitempty"`    // Maps to UNIQUE
	Dimension int    `json:"dimension,omitempty"` // Vector dimensions (1..4096), used when Type == "vector"
}

// Collection represents a user-defined table registered in _collections.
type Collection struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Fields    []FieldDef `json:"fields"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreateCollectionRequest is the payload accepted by POST /api/collections.
type CreateCollectionRequest struct {
	Name   string     `json:"name"`
	Fields []FieldDef `json:"fields"`
}

// SchemaExport represents a portable snapshot of all collection schemas for environment migration.
type SchemaExport struct {
	Version     string                    `json:"version"`
	ExportedAt  time.Time                 `json:"exported_at"`
	Collections []CreateCollectionRequest `json:"collections"`
}

// IsValidIdentifier reports whether name is a safe, non-system SQL identifier.
func IsValidIdentifier(name string) bool {
	return identifierRegex.MatchString(name)
}

// ToPostgresType maps a Microbase field definition to its corresponding PostgreSQL column type.
func (f FieldDef) ToPostgresType() (string, error) {
	switch f.Type {
	case "text":
		return "TEXT", nil
	case "integer":
		return "BIGINT", nil
	case "float":
		return "DOUBLE PRECISION", nil
	case "boolean":
		return "BOOLEAN", nil
	case "json":
		return "JSONB", nil
	case "timestamptz":
		return "TIMESTAMPTZ", nil
	case "vector":
		if f.Dimension <= 0 || f.Dimension > 4096 {
			return "", fmt.Errorf("vector field %q must have dimension between 1 and 4096", f.Name)
		}
		return fmt.Sprintf("vector(%d)", f.Dimension), nil
	default:
		return "", fmt.Errorf("unsupported field type %q", f.Type)
	}
}
