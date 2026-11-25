package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// CollectionAuthController provides register, auth-with-password, and auth-refresh for ANY Type: Auth collection.
type CollectionAuthController struct {
	pool        *pgxpool.Pool
	collections *services.CollectionService
	auth        *services.AuthService
}

func NewCollectionAuthController(pool *pgxpool.Pool, colSvc *services.CollectionService, authSvc *services.AuthService) *CollectionAuthController {
	return &CollectionAuthController{pool: pool, collections: colSvc, auth: authSvc}
}

func authTableName(col *models.Collection) string {
	if col.Name == "users" {
		return "_users"
	}
	return col.Name
}

// Register handles POST /api/collections/:name/register for any Auth collection.
func (h *CollectionAuthController) Register(c echo.Context) error {
	col, err := h.collections.GetByName(c.Request().Context(), c.Param("name"))
	if err != nil {
		return response.Error(c, http.StatusNotFound, "COLLECTION_NOT_FOUND", "collection not found")
	}
	if col.Type != "auth" && col.Name != "users" {
		return response.Error(c, http.StatusBadRequest, "NOT_AUTH_COLLECTION", "collection is not an auth collection")
	}

	var raw map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	claims := middlewares.GetAuthClaims(c)
	if err := services.EvalCreateRule(col, claims, raw); err != nil {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by create api rule")
	}

	email := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw["email"])))
	password, _ := raw["password"].(string)
	if !strings.Contains(email, "@") || len(password) < 8 {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "valid email and password (min 8 chars) required")
	}

	hash, err := services.HashPassword(password)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	tokenKey := services.GenerateFromPattern("[a-zA-Z0-9]{50}")
	cols := []string{"email", "password_hash", "token_key"}
	placeholders := []string{"$1", "$2", "$3"}
	args := []any{email, hash, tokenKey}
	idx := 4

	if col.IDType == "text" {
		idVal := strings.TrimSpace(fmt.Sprint(raw["id"]))
		if idVal == "" || idVal == "<nil>" {
			pat := col.IDAutogeneratePattern
			if pat == "" {
				pat = "[a-z0-9]{15}"
			}
			idVal = services.GenerateFromPattern(pat)
		}
		cols = append(cols, "id")
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, idVal)
		idx++
	}

	allowed := make(map[string]bool, len(col.Fields)+2)
	for _, f := range col.Fields {
		allowed[f.Name] = true
	}
	allowed["email_visibility"] = true
	if claims != nil && claims.Role == "superuser" {
		allowed["verified"] = true
	}

	for k, v := range raw {
		if !allowed[k] || k == "id" || k == "email" || k == "password" || k == "password_hash" || k == "token_key" || v == nil {
			continue
		}
		cols = append(cols, fmt.Sprintf(`"%s"`, k))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, v)
		idx++
	}

	query := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s) RETURNING *`,
		authTableName(col), strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	rows, err := h.pool.Query(c.Request().Context(), query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return response.Error(c, http.StatusConflict, "EMAIL_EXISTS", "email already registered")
		}
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer rows.Close()

	if !rows.Next() {
		var pgErr *pgconn.PgError
		if errors.As(rows.Err(), &pgErr) && pgErr.Code == "23505" {
			return response.Error(c, http.StatusConflict, "EMAIL_EXISTS", "email already registered")
		}
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create auth record")
	}
	descs := rows.FieldDescriptions()
	vals, _ := rows.Values()
	rec := make(models.Record, len(descs))
	for i, fd := range descs {
		name := string(fd.Name)
		if name == "password_hash" || name == "token_key" {
			continue
		}
		rec[name] = vals[i]
	}

	token, err := h.auth.GenerateCollectionToken(col.Name, fmt.Sprint(rec["id"]), email, "user")
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.Created(c, map[string]any{"token": token, "record": rec})
}

// AuthWithPassword handles POST /api/collections/:name/auth-with-password for any Auth collection.
func (h *CollectionAuthController) AuthWithPassword(c echo.Context) error {
	col, err := h.collections.GetByName(c.Request().Context(), c.Param("name"))
	if err != nil {
		return response.Error(c, http.StatusNotFound, "COLLECTION_NOT_FOUND", "collection not found")
	}
	if col.Type != "auth" && col.Name != "users" {
		return response.Error(c, http.StatusBadRequest, "NOT_AUTH_COLLECTION", "collection is not an auth collection")
	}

	var req struct {
		Identity string `json:"identity"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(req.Identity))
	}
	if email == "" || req.Password == "" {
		return response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	}

	rows, err := h.pool.Query(c.Request().Context(), fmt.Sprintf(`SELECT * FROM "%s" WHERE email = $1`, authTableName(col)), email)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer rows.Close()

	if !rows.Next() {
		return response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	}
	descs := rows.FieldDescriptions()
	vals, _ := rows.Values()
	rec := make(models.Record, len(descs))
	var passHash string
	for i, fd := range descs {
		name := string(fd.Name)
		if name == "password_hash" {
			passHash, _ = vals[i].(string)
			continue
		}
		if name == "token_key" {
			continue
		}
		rec[name] = vals[i]
	}

	ok, err := services.VerifyPassword(req.Password, passHash)
	if err != nil || !ok {
		return response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	}

	role := "user"
	if r, ok := rec["role"].(string); ok && r != "" {
		role = r
	}
	token, err := h.auth.GenerateCollectionToken(col.Name, fmt.Sprint(rec["id"]), email, role)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"token": token, "record": rec})
}

// AuthRefresh handles POST /api/collections/:name/auth-refresh.
func (h *CollectionAuthController) AuthRefresh(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil {
		return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
	}
	col, err := h.collections.GetByName(c.Request().Context(), c.Param("name"))
	if err != nil {
		return response.Error(c, http.StatusNotFound, "COLLECTION_NOT_FOUND", "collection not found")
	}
	idStr := claims.RecordID
	if idStr == "" {
		idStr = fmt.Sprint(claims.UserID)
	}
	token, err := h.auth.GenerateCollectionToken(col.Name, idStr, claims.Email, claims.Role)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"token": token, "collection": col.Name, "id": idStr, "email": claims.Email})
}
