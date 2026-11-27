package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/hooks"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/realtime"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrInvalidInput   = errors.New("invalid record input")
)

// RecordService provides dynamic CRUD and search operations for registered collections.
type RecordService struct {
	pool        *pgxpool.Pool
	collections *CollectionService
	hub         *realtime.Hub
	webhooks    *WebhookService
	hooks       *hooks.Engine
}

// SetHooks attaches an optional embedded JS hooks engine to RecordService.
func (s *RecordService) SetHooks(h *hooks.Engine) { s.hooks = h }

func NewRecordService(pool *pgxpool.Pool, colSvc *CollectionService, hub *realtime.Hub, webhooks *WebhookService) *RecordService {
	return &RecordService{pool: pool, collections: colSvc, hub: hub, webhooks: webhooks}
}

// List queries records with pagination, sorting, exact filters, and full-text search.
func (s *RecordService) List(ctx context.Context, colName string, opts models.ListOptions) (*models.ListResult, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}
	opts.Normalize()
	allowed := allowedFieldMap(col)

	whereSQL, args, nextIdx, err := buildWhereClause(col, opts, allowed)
	if err != nil {
		return nil, err
	}

	var total int64 = -1
	if !opts.SkipTotal {
		countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"%s`, col.Name, whereSQL)
		if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
			return nil, fmt.Errorf("count records: %w", err)
		}
	}

	orderSQL, err := buildOrderBy(opts.Sort, allowed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	selectSQL := fmt.Sprintf(`SELECT * FROM "%s"%s ORDER BY %s LIMIT $%d OFFSET $%d`,
		col.Name, whereSQL, orderSQL, nextIdx, nextIdx+1)
	args = append(args, opts.PerPage, opts.Offset())

	rows, err := s.pool.Query(ctx, selectSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query records: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}

	return &models.ListResult{
		Page:       opts.Page,
		PerPage:    opts.PerPage,
		TotalItems: total,
		Items:      items,
	}, nil
}

// GetByID fetches a single record by its ID while enforcing the collection's row-level "view" API rule.
func (s *RecordService) GetByID(ctx context.Context, colName string, rawID string, claims *AuthClaims) (models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}

	idVal, idErr := parseRecordID(col, rawID)
	if idErr != nil {
		return nil, idErr
	}

	ruleSQL, ruleArgs, _, ruleErr := CompileAPIRule(col, "view", claims, nil, 2)
	if ruleErr != nil {
		return nil, ruleErr
	}

	query := fmt.Sprintf(`SELECT * FROM "%s" WHERE "id" = $1`, col.Name)
	args := []any{idVal}
	if ruleSQL != "" {
		query += " AND (" + ruleSQL + ")"
		args = append(args, ruleArgs...)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get record: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if ruleSQL != "" && s.recordExists(ctx, col.Name, idVal) {
			return nil, ErrForbidden
		}
		return nil, ErrRecordNotFound
	}
	return items[0], nil
}

func (s *RecordService) recordExists(ctx context.Context, tableName string, idVal any) bool {
	var one int
	return s.pool.QueryRow(ctx, fmt.Sprintf(`SELECT 1 FROM "%s" WHERE "id" = $1`, tableName), idVal).Scan(&one) == nil
}

// Create validates input against the collection schema and inserts a new record.
func (s *RecordService) Create(ctx context.Context, colName string, data models.Record, claims *AuthClaims) (models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}
	allowed := allowedFieldMap(col)
	if (col.Type == "auth" || col.Name == "users") && (claims == nil || claims.Role != "superuser") {
		if _, ok := data["role"]; ok {
			return nil, ErrForbidden
		}
		if _, ok := data["verified"]; ok {
			return nil, ErrForbidden
		}
	}

	if data == nil {
		data = models.Record{}
	}
	// Auto-generate PocketBase-style random string ID for text-ID collections.
	if col.IDType == "text" {
		allowed["id"] = models.FieldDef{Name: "id", Type: "text"}
		if rawID, exists := data["id"]; !exists || rawID == nil || strings.TrimSpace(fmt.Sprint(rawID)) == "" {
			pattern := col.IDAutogeneratePattern
			if strings.TrimSpace(pattern) == "" {
				pattern = "[a-z0-9]{15}"
			}
			data["id"] = GenerateFromPattern(pattern)
		}
	}
	// Apply PocketBase-style AutogeneratePattern on missing text fields and autodate (on_create) before required checks.
	now := time.Now().UTC()
	for _, f := range col.Fields {
		if f.Type == "text" && f.AutogeneratePattern != "" {
			val, exists := data[f.Name]
			if !exists || val == nil || strings.TrimSpace(fmt.Sprint(val)) == "" {
				data[f.Name] = GenerateFromPattern(f.AutogeneratePattern)
			}
		}
		if f.Type == "autodate" {
			// Autodate fields are server-managed (read-only): never allow client override or clearing.
			delete(data, f.Name)
			if f.OnCreate || !f.OnUpdate {
				data[f.Name] = now
			}
		}
	}

	if s.hooks != nil {
		if err := s.hooks.RunBefore("beforeCreate", col.Name, map[string]any{"collection": col.Name, "data": data}); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}
	if err := EvalCreateRuleWithDB(ctx, s.pool, col, claims, data); err != nil {
		return nil, err
	}

	for _, f := range col.Fields {
		if f.Required {
			if val, exists := data[f.Name]; !exists || val == nil {
				return nil, fmt.Errorf("%w: field %q is required", ErrInvalidInput, f.Name)
			}
		}
	}

	cols, placeholders, args := make([]string, 0, len(data)), make([]string, 0, len(data)), make([]any, 0, len(data))
	idx := 1
	for key, raw := range data {
		field, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalidInput, key)
		}
		val, err := coerceInputValue(field, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		cols = append(cols, fmt.Sprintf(`"%s"`, key))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, val)
		idx++
	}

	insertSQL := fmt.Sprintf(`INSERT INTO "%s" DEFAULT VALUES RETURNING *`, col.Name)
	if len(cols) > 0 {
		insertSQL = fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s) RETURNING *`,
			col.Name, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	}

	rows, err := s.pool.Query(ctx, insertSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("insert record: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	s.publish("create", col.Name, items[0])
	return items[0], nil
}

// Update modifies specified fields on an existing record and updates updated_at.
func (s *RecordService) Update(ctx context.Context, colName string, rawID string, data models.Record, claims *AuthClaims) (models.Record, error) {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return nil, err
	}
	allowed := allowedFieldMap(col)
	if (col.Type == "auth" || col.Name == "users") && (claims == nil || claims.Role != "superuser") {
		if _, ok := data["role"]; ok {
			return nil, ErrForbidden
		}
		if _, ok := data["verified"]; ok {
			return nil, ErrForbidden
		}
	}
	if data == nil {
		data = models.Record{}
	}
	// Autodate fields are strictly server-managed: strip any client-supplied value,
	// preserve create-only autodate fields, and refresh on_update fields automatically.
	now := time.Now().UTC()
	for _, f := range col.Fields {
		if f.Type == "autodate" {
			delete(data, f.Name)
			if f.OnUpdate {
				data[f.Name] = now
			}
		}
	}

	idVal, idErr := parseRecordID(col, rawID)
	if idErr != nil {
		return nil, idErr
	}

	// Pre-check static/update API rule before running JS hooks so unauthorized callers cannot trigger beforeUpdate.
	if _, _, _, preRuleErr := CompileAPIRule(col, "update", claims, data, 1); preRuleErr != nil {
		return nil, preRuleErr
	}

	if s.hooks != nil {
		if err := s.hooks.RunBefore("beforeUpdate", col.Name, map[string]any{"collection": col.Name, "id": rawID, "data": data}); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		// Re-enforce protected fields after hook mutations for non-superusers.
		if (col.Type == "auth" || col.Name == "users") && (claims == nil || claims.Role != "superuser") {
			delete(data, "role")
			delete(data, "verified")
		}
	}

	setClauses, args := make([]string, 0, len(data)+1), make([]any, 0, len(data)+1)
	idx := 1
	for key, raw := range data {
		field, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalidInput, key)
		}
		val, err := coerceInputValue(field, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		setClauses = append(setClauses, fmt.Sprintf(`"%s" = $%d`, key, idx))
		args = append(args, val)
		idx++
	}
	if len(setClauses) == 0 {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}

	setClauses = append(setClauses, `"updated_at" = NOW()`)
	args = append(args, idVal)

	ruleSQL, ruleArgs, _, ruleErr := CompileAPIRule(col, "update", claims, data, idx+1)
	if ruleErr != nil {
		return nil, ruleErr
	}

	wherePart := fmt.Sprintf(`"id" = $%d`, idx)
	if ruleSQL != "" {
		wherePart += " AND (" + ruleSQL + ")"
		args = append(args, ruleArgs...)
	}

	updateSQL := fmt.Sprintf(`UPDATE "%s" SET %s WHERE %s RETURNING *`,
		col.Name, strings.Join(setClauses, ", "), wherePart)

	rows, err := s.pool.Query(ctx, updateSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("update record: %w", err)
	}
	defer rows.Close()

	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if ruleSQL != "" && s.recordExists(ctx, col.Name, idVal) {
			return nil, ErrForbidden
		}
		return nil, ErrRecordNotFound
	}
	s.publish("update", col.Name, items[0])
	return items[0], nil
}

// Delete removes a record by ID while enforcing the collection's row-level "delete" API rule.
func (s *RecordService) Delete(ctx context.Context, colName string, rawID string, claims *AuthClaims) error {
	col, err := s.collections.GetByName(ctx, colName)
	if err != nil {
		return err
	}

	idVal, idErr := parseRecordID(col, rawID)
	if idErr != nil {
		return idErr
	}

	ruleSQL, ruleArgs, _, ruleErr := CompileAPIRule(col, "delete", claims, nil, 2)
	if ruleErr != nil {
		return ruleErr
	}
	if s.hooks != nil {
		if err := s.hooks.RunBefore("beforeDelete", col.Name, map[string]any{"collection": col.Name, "id": rawID}); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}

	query := fmt.Sprintf(`DELETE FROM "%s" WHERE "id" = $1`, col.Name)
	args := []any{idVal}
	if ruleSQL != "" {
		query += " AND (" + ruleSQL + ")"
		args = append(args, ruleArgs...)
	}

	rows, err := s.pool.Query(ctx, query+" RETURNING *", args...)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	deleted, err := scanRows(rows)
	rows.Close()
	if err != nil {
		return err
	}
	if len(deleted) == 0 {
		if ruleSQL != "" && s.recordExists(ctx, col.Name, idVal) {
			return ErrForbidden
		}
		return ErrRecordNotFound
	}
	payload := map[string]any{"id": idVal}
	if s.hub != nil {
		s.hub.Publish(realtime.Event{Action: "delete", Collection: col.Name, Record: payload, Snapshot: deleted[0]})
	}
	if s.webhooks != nil {
		s.webhooks.DispatchAsync(col.Name, "delete", payload)
	}
	return nil
}
func (s *RecordService) publish(action, colName string, record any) {
	if s.hub != nil {
		s.hub.Publish(realtime.Event{
			Action:     action,
			Collection: colName,
			Record:     record,
		})
	}
	if s.webhooks != nil {
		s.webhooks.DispatchAsync(colName, action, record)
	}
}

func parseRecordID(col *models.Collection, rawID string) (any, error) {
	if col.IDType == "text" {
		if strings.TrimSpace(rawID) == "" {
			return nil, fmt.Errorf("%w: record id cannot be empty", ErrInvalidInput)
		}
		return rawID, nil
	}
	n, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: record id must be an integer", ErrInvalidInput)
	}
	return n, nil
}
