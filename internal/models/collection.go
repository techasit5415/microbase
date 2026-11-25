package models

import (
	"fmt"
	"regexp"
	"time"
)

// identifierRegex enforces PostgreSQL-safe lowercase identifiers up to 63 bytes.
// Leading underscores are disallowed to reserve system tables (e.g., _collections, _users).
var identifierRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// FieldDef defines the schema and PocketBase-compatible options for a single column within a collection.
type FieldDef struct {
	Name         string   `json:"name"`
	OriginalName string   `json:"original_name,omitempty"` // Used on PATCH to rename an existing column
	Type         string   `json:"type"`                    // text, editor, number, integer, float, boolean, email, url, datetime, timestamptz, autodate, file, relation, select, json, geopoint, vector
	Required     bool     `json:"required,omitempty"`      // Maps to NOT NULL
	Unique       bool     `json:"unique,omitempty"`        // Maps to UNIQUE
	Presentable  bool     `json:"presentable,omitempty"`   // Display label in relations & previews
	Hidden       bool     `json:"hidden,omitempty"`        // Omitted from non-superuser API responses
	HelpText     string   `json:"help_text,omitempty"`     // Helper description shown under form input

	// Relation options
	CollectionName string `json:"collection_name,omitempty"` // Target collection for relation fields
	CascadeDelete  bool   `json:"cascade_delete,omitempty"`  // ON DELETE CASCADE vs SET NULL

	// Select / File / Relation multiplicity
	MaxSelect int      `json:"max_select,omitempty"` // 1 = Single, >1 = Multiple
	Values    []string `json:"values,omitempty"`     // Allowed choices for select fields

	// File options
	MimeTypes []string `json:"mime_types,omitempty"` // Allowed MIME types
	Thumbs    []string `json:"thumbs,omitempty"`     // Thumbnail sizes e.g. ["50x50", "480x720"]
	MaxSize   int64    `json:"max_size,omitempty"`   // Max size in bytes
	Protected bool     `json:"protected,omitempty"`  // Require View rule / auth to download

	// Text / Number / Email / URL / Autodate options
	Min           *float64 `json:"min,omitempty"`            // Min length (text) or min value (number)
	Max           *float64 `json:"max,omitempty"`            // Max length (text) or max value (number)
	Pattern       string   `json:"pattern,omitempty"`        // Regex pattern for text fields
	OnlyInt       bool     `json:"only_int,omitempty"`       // Restrict number field to integers
	OnlyDomains   []string `json:"only_domains,omitempty"`   // Allowed domains for email/url
	ExceptDomains []string `json:"except_domains,omitempty"` // Blocked domains for email/url
	OnCreate      bool     `json:"on_create,omitempty"`      // Autodate: set on create
	OnUpdate      bool     `json:"on_update,omitempty"`      // Autodate: update on modify

	// Vector options
	Dimension int `json:"dimension,omitempty"` // Vector dimensions (1..4096)
}

// IndexDef represents a PostgreSQL B-Tree or UNIQUE index on one or more columns.
type IndexDef struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
}

// Collection represents a user-defined table registered in _collections.
type Collection struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Fields    []FieldDef        `json:"fields"`
	APIRules  map[string]string `json:"api_rules,omitempty"`
	Indexes   []IndexDef        `json:"indexes,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// CreateCollectionRequest is the payload accepted by POST /api/collections.
type CreateCollectionRequest struct {
	Name     string            `json:"name"`
	Fields   []FieldDef        `json:"fields"`
	APIRules map[string]string `json:"api_rules,omitempty"`
	Indexes  []IndexDef        `json:"indexes,omitempty"`
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

// ToPostgresType maps a Microbase/PocketBase field definition to its PostgreSQL column type.
func (f FieldDef) ToPostgresType() (string, error) {
	switch f.Type {
	case "text", "editor", "email", "url", "file", "select":
		if (f.Type == "select" || f.Type == "file") && f.MaxSelect > 1 {
			return "JSONB", nil
		}
		return "TEXT", nil
	case "relation":
		if f.MaxSelect > 1 {
			return "JSONB", nil
		}
		return "BIGINT", nil
	case "integer":
		return "BIGINT", nil
	case "number", "float":
		if f.OnlyInt {
			return "BIGINT", nil
		}
		return "DOUBLE PRECISION", nil
	case "boolean":
		return "BOOLEAN", nil
	case "json", "geopoint":
		return "JSONB", nil
	case "datetime", "timestamptz":
		return "TIMESTAMPTZ", nil
	case "autodate":
		return "TIMESTAMPTZ DEFAULT NOW()", nil
	case "vector":
		if f.Dimension <= 0 || f.Dimension > 4096 {
			return "", fmt.Errorf("vector field %q must have dimension between 1 and 4096", f.Name)
		}
		return fmt.Sprintf("vector(%d)", f.Dimension), nil
	default:
		return "", fmt.Errorf("unsupported field type %q", f.Type)
	}
}
