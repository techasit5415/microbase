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
	Name         string   `json:"name"`
	OriginalName string   `json:"original_name,omitempty"` // Used on PATCH to rename an existing column
	Type         string   `json:"type"`                    // text, editor, number, integer, float, boolean, email, url, datetime, timestamptz, autodate, file, relation, select, json, geopoint, vector
	Required     bool     `json:"required,omitempty"`      // Maps to NOT NULL
	Unique       bool     `json:"unique,omitempty"`        // Maps to UNIQUE
	Presentable  bool     `json:"presentable,omitempty"`   // Display label in relations & previews
	Hidden       bool     `json:"hidden,omitempty"`        // Omitted from non-superuser API responses
	HelpText     string   `json:"help_text,omitempty"`     // Helper description shown under form input
	Dimension    int      `json:"dimension,omitempty"`     // Vector dimensions (1..4096), used when Type == "vector"
	MaxSelect    int      `json:"max_select,omitempty"`    // 1 = Single, >1 = Multiple (for file/relation/select)
	MimeTypes    []string `json:"mime_types,omitempty"`    // Allowed MIME types for file fields
	Thumbs       []string `json:"thumbs,omitempty"`        // Thumbnail sizes e.g. ["50x50", "480x720"]
	MaxSize      int64    `json:"max_size,omitempty"`      // Max file size in bytes (0 = default 5MB)
	Protected    bool     `json:"protected,omitempty"`     // Require View rule / auth to download file
}

// IndexDef represents a PostgreSQL B-Tree or UNIQUE index on one or more columns.
type IndexDef struct {
	Name    string   `json:"name"`             // e.g. "idx_tgn94bxgmg"
	Columns []string `json:"columns"`          // Ordered list of indexed column names
	Unique  bool     `json:"unique,omitempty"` // true => CREATE UNIQUE INDEX
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
		return "TEXT", nil
	case "integer", "relation":
		return "BIGINT", nil
	case "number", "float":
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
