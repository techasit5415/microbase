package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/techasit5415/microbase/internal/models"
)

// ExportSchema returns a portable JSON snapshot of all collection schemas.
func (s *CollectionService) ExportSchema(ctx context.Context) (*models.SchemaExport, error) {
	cols, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	exported := make([]models.CreateCollectionRequest, 0, len(cols))
	for _, c := range cols {
		exported = append(exported, models.CreateCollectionRequest{
			Name:                  c.Name,
			Type:                  c.Type,
			ViewQuery:             c.ViewQuery,
			IDType:                c.IDType,
			IDAutogeneratePattern: c.IDAutogeneratePattern,
			Fields:                c.Fields,
			APIRules:              c.APIRules,
			Indexes:               c.Indexes,
		})
	}

	return &models.SchemaExport{
		Version:     "1.0",
		ExportedAt:  time.Now().UTC(),
		Collections: exported,
	}, nil
}

// ImportSchema creates any missing collections from a SchemaExport snapshot (idempotent).
// Collections are topologically ordered so relation targets are created before referencing tables.
func (s *CollectionService) ImportSchema(ctx context.Context, payload models.SchemaExport) (created []string, skipped []string, err error) {
	created = make([]string, 0)
	skipped = make([]string, 0)

	ordered := orderCollectionsByRelationDeps(payload.Collections)
	for _, colReq := range ordered {
		_, createErr := s.Create(ctx, colReq)
		if errors.Is(createErr, ErrCollectionExists) {
			skipped = append(skipped, colReq.Name)
			continue
		}
		if createErr != nil {
			return created, skipped, fmt.Errorf("import %q: %w", colReq.Name, createErr)
		}
		created = append(created, colReq.Name)
	}
	return created, skipped, nil
}

// orderCollectionsByRelationDeps sorts collections so that any collection referenced by a
// single-value relation FK is created before the collection that references it.
func orderCollectionsByRelationDeps(cols []models.CreateCollectionRequest) []models.CreateCollectionRequest {
	if len(cols) <= 1 {
		return cols
	}
	byName := make(map[string]models.CreateCollectionRequest, len(cols))
	orderIdx := make(map[string]int, len(cols))
	for i, c := range cols {
		byName[c.Name] = c
		orderIdx[c.Name] = i
	}

	deps := make(map[string]map[string]bool, len(cols))
	dependents := make(map[string][]string, len(cols))
	inDeg := make(map[string]int, len(cols))

	for _, c := range cols {
		deps[c.Name] = make(map[string]bool)
		for _, f := range c.Fields {
			if f.Type == "relation" && f.MaxSelect <= 1 && f.CollectionName != "" && f.CollectionName != c.Name {
				if _, inBatch := byName[f.CollectionName]; inBatch && !deps[c.Name][f.CollectionName] {
					deps[c.Name][f.CollectionName] = true
					dependents[f.CollectionName] = append(dependents[f.CollectionName], c.Name)
				}
			}
		}
		inDeg[c.Name] = len(deps[c.Name])
	}

	queue := make([]string, 0, len(cols))
	for _, c := range cols {
		if inDeg[c.Name] == 0 {
			queue = append(queue, c.Name)
		}
	}

	ordered := make([]models.CreateCollectionRequest, 0, len(cols))
	visited := make(map[string]bool, len(cols))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		ordered = append(ordered, byName[curr])
		for _, dep := range dependents[curr] {
			inDeg[dep]--
			if inDeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// Append any remaining collections (e.g. mutual cycles) in their original order.
	for _, c := range cols {
		if !visited[c.Name] {
			ordered = append(ordered, c)
		}
	}
	return ordered
}

// pbCollection models a PocketBase pb_schema.json collection entry (v0.22 and v0.23+).
type pbCollection struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	System     bool       `json:"system"`
	ViewQuery  string     `json:"viewQuery"`
	ListRule   *string    `json:"listRule"`
	ViewRule   *string    `json:"viewRule"`
	CreateRule *string    `json:"createRule"`
	UpdateRule *string    `json:"updateRule"`
	DeleteRule *string    `json:"deleteRule"`
	Fields     []pbField  `json:"fields"`
	Schema     []pbField  `json:"schema"` // PocketBase <= v0.22
	Indexes    []string   `json:"indexes"`
}

type pbField struct {
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	System              bool     `json:"system"`
	Required            bool     `json:"required"`
	Presentable         bool     `json:"presentable"`
	Hidden              bool     `json:"hidden"`
	Help                string   `json:"help"`
	CollectionID        string   `json:"collectionId"`
	CascadeDelete       bool     `json:"cascadeDelete"`
	MaxSelect           int      `json:"maxSelect"`
	Values              []string `json:"values"`
	MimeTypes           []string `json:"mimeTypes"`
	Thumbs              []string `json:"thumbs"`
	MaxSize             int64    `json:"maxSize"`
	Protected           bool     `json:"protected"`
	Min                 any      `json:"min"`
	Max                 any      `json:"max"`
	Pattern             string   `json:"pattern"`
	AutogeneratePattern string   `json:"autogeneratePattern"`
	OnlyInt             bool     `json:"onlyInt"`
	OnlyDomains         []string `json:"onlyDomains"`
	ExceptDomains       []string `json:"exceptDomains"`
	OnCreate            bool     `json:"onCreate"`
	OnUpdate            bool     `json:"onUpdate"`
	// PocketBase <= v0.22 nested options fallback
	Options map[string]any `json:"options"`
}

var pbIndexRegex = regexp.MustCompile(`(?i)^\s*CREATE\s+(UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?[` + "`" + `"]?([a-zA-Z0-9_]+)[` + "`" + `"]?\s+ON\s+[` + "`" + `"]?([a-zA-Z0-9_]+)[` + "`" + `"]?\s*\(([^)]+)\)\s*$`)

// ParsePocketBaseSchema converts a PocketBase pb_schema.json array into a Microbase SchemaExport.
func ParsePocketBaseSchema(raw []byte) (models.SchemaExport, error) {
	var pbCols []pbCollection
	if err := json.Unmarshal(raw, &pbCols); err != nil {
		return models.SchemaExport{}, err
	}

	idToName := map[string]string{"_pb_users_auth_": "users"}
	for _, c := range pbCols {
		if c.ID != "" && c.Name != "" {
			idToName[c.ID] = c.Name
		}
	}

	authBuiltInFields := map[string]bool{
		"email":            true,
		"password":         true,
		"password_hash":    true,
		"tokenKey":         true,
		"token_key":        true,
		"emailVisibility":  true,
		"email_visibility": true,
		"verified":         true,
	}

	collections := make([]models.CreateCollectionRequest, 0, len(pbCols))
	for _, c := range pbCols {
		if c.System || strings.HasPrefix(c.Name, "_") || c.Name == "" {
			continue
		}

		colType := c.Type
		if colType != "base" && colType != "view" && colType != "auth" {
			colType = "base"
		}

		srcFields := c.Fields
		if len(srcFields) == 0 && len(c.Schema) > 0 {
			srcFields = c.Schema
		}

		idType := "text"
		idPattern := "[a-z0-9]{15}"
		fields := make([]models.FieldDef, 0, len(srcFields))
		validCols := map[string]bool{"id": true, "created_at": true, "updated_at": true}
		if colType == "auth" {
			validCols["email"] = true
			validCols["verified"] = true
		}

		for _, f := range srcFields {
			if f.Name == "id" {
				if f.Type == "text" {
					idType = "text"
					if f.AutogeneratePattern != "" {
						idPattern = f.AutogeneratePattern
					}
				}
				continue
			}
			if f.System || reservedColumns[f.Name] || f.Type == "password" {
				continue
			}
			if colType == "auth" && authBuiltInFields[f.Name] {
				continue
			}

			mbType := mapPocketBaseFieldType(f.Type)
			if mbType == "" {
				continue
			}

			fd := models.FieldDef{
				Name:                f.Name,
				Type:                mbType,
				Required:            f.Required,
				Presentable:         f.Presentable,
				Hidden:              f.Hidden,
				HelpText:            f.Help,
				CascadeDelete:       f.CascadeDelete,
				MaxSelect:           f.MaxSelect,
				Values:              f.Values,
				MimeTypes:           f.MimeTypes,
				Thumbs:              f.Thumbs,
				MaxSize:             f.MaxSize,
				Protected:           f.Protected,
				Pattern:             f.Pattern,
				AutogeneratePattern: f.AutogeneratePattern,
				OnlyInt:             f.OnlyInt,
				OnlyDomains:         f.OnlyDomains,
				ExceptDomains:       f.ExceptDomains,
				OnCreate:            f.OnCreate,
				OnUpdate:            f.OnUpdate,
			}

			if v, ok := f.Min.(float64); ok && v != 0 {
				val := v
				fd.Min = &val
			}
			if v, ok := f.Max.(float64); ok && v != 0 {
				val := v
				fd.Max = &val
			}

			// Support PocketBase <= v0.22 nested options map.
			if len(f.Options) > 0 {
				applyLegacyPBOptions(&fd, f.Options)
			}

			if fd.Type == "relation" {
				targetID := f.CollectionID
				if targetID == "" && f.Options != nil {
					if v, ok := f.Options["collectionId"].(string); ok {
						targetID = v
					}
				}
				targetName := idToName[targetID]
				if targetName == "" {
					targetName = targetID
				}
				// If target is a skipped PocketBase system table (e.g. _superusers), store as text instead of broken FK.
				if targetName == "" || strings.HasPrefix(targetName, "_") {
					fd.Type = "text"
					fd.CollectionName = ""
					fd.MaxSelect = 0
				} else {
					fd.CollectionName = targetName
				}
			}

			if fd.Type == "select" && len(fd.Values) == 0 {
				fd.Type = "text"
			}

			fields = append(fields, fd)
			validCols[fd.Name] = true
		}

		apiRules := make(map[string]string)
		if c.ListRule != nil {
			apiRules["list"] = *c.ListRule
		}
		if c.ViewRule != nil {
			apiRules["view"] = *c.ViewRule
		}
		if c.CreateRule != nil {
			apiRules["create"] = *c.CreateRule
		}
		if c.UpdateRule != nil {
			apiRules["update"] = *c.UpdateRule
		}
		if c.DeleteRule != nil {
			apiRules["delete"] = *c.DeleteRule
		}

		indexes := parsePocketBaseIndexes(c.Indexes, validCols)

		collections = append(collections, models.CreateCollectionRequest{
			Name:                  c.Name,
			Type:                  colType,
			ViewQuery:             c.ViewQuery,
			IDType:                idType,
			IDAutogeneratePattern: idPattern,
			Fields:                fields,
			APIRules:              apiRules,
			Indexes:               indexes,
		})
	}

	return models.SchemaExport{
		Version:     "1.0",
		ExportedAt:  time.Now().UTC(),
		Collections: collections,
	}, nil
}

func mapPocketBaseFieldType(pbType string) string {
	switch pbType {
	case "bool":
		return "boolean"
	case "date":
		return "datetime"
	case "text", "editor", "number", "email", "url", "file", "relation", "select", "json", "geopoint", "autodate":
		return pbType
	default:
		return "text"
	}
}

func applyLegacyPBOptions(fd *models.FieldDef, opts map[string]any) {
	if v, ok := opts["maxSelect"].(float64); ok && fd.MaxSelect == 0 {
		fd.MaxSelect = int(v)
	}
	if v, ok := opts["cascadeDelete"].(bool); ok && !fd.CascadeDelete {
		fd.CascadeDelete = v
	}
	if v, ok := opts["values"].([]any); ok && len(fd.Values) == 0 {
		for _, item := range v {
			if s, ok := item.(string); ok {
				fd.Values = append(fd.Values, s)
			}
		}
	}
}

func parsePocketBaseIndexes(rawIndexes []string, validCols map[string]bool) []models.IndexDef {
	out := make([]models.IndexDef, 0, len(rawIndexes))
	for _, raw := range rawIndexes {
		m := pbIndexRegex.FindStringSubmatch(raw)
		if len(m) != 5 {
			continue // Skip partial/expression indexes (e.g. WHERE ...) not representable in IndexDef
		}
		idxName := m[2]
		if !models.IsValidIdentifier(idxName) {
			continue
		}
		rawCols := strings.Split(m[4], ",")
		cols := make([]string, 0, len(rawCols))
		ok := true
		for _, rc := range rawCols {
			token := strings.TrimSpace(rc)
			// Strip optional ASC/DESC
			parts := strings.Fields(token)
			if len(parts) == 0 {
				ok = false
				break
			}
			col := strings.Trim(parts[0], "`\" ")
			if !validCols[col] || !models.IsValidIdentifier(col) {
				ok = false
				break
			}
			cols = append(cols, col)
		}
		if !ok || len(cols) == 0 {
			continue
		}
		out = append(out, models.IndexDef{
			Name:    idxName,
			Columns: cols,
			Unique:  strings.TrimSpace(m[1]) != "",
		})
	}
	return out
}
