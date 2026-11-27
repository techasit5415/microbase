package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/models"
)

const tokenTTL = 24 * time.Hour

var (
	ErrEmailExists        = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrMissingJWTSecret   = errors.New("JWT_SECRET is not configured")
)

// AuthClaims holds the JWT payload for authenticated users.
type AuthClaims struct {
	Scope      string `json:"scope,omitempty"`
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	RecordID   string `json:"record_id,omitempty"`
	Collection string `json:"collection,omitempty"`
	jwt.RegisteredClaims
}

// AuthService manages user registration, login, and JWT issuance/verification.
type AuthService struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
}

// NewAuthService constructs an AuthService.
func NewAuthService(pool *pgxpool.Pool, jwtSecret string) *AuthService {
	return &AuthService{
		pool:      pool,
		jwtSecret: []byte(jwtSecret),
	}
}

// Register creates a new user with an Argon2id password hash and returns a signed JWT.
func (s *AuthService) Register(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {
	return s.RegisterWithExtras(ctx, req.Email, req.Password, nil, nil)
}

// RegisterWithExtras creates a new user with email, password, and optional role/verified/email_visibility/custom users fields.
func (s *AuthService) RegisterWithExtras(ctx context.Context, rawEmail, password string, extras map[string]any, claims *AuthClaims) (*models.AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	if !strings.Contains(email, "@") || len(password) < 8 {
		return nil, fmt.Errorf("%w: valid email and password (min 8 chars) required", ErrInvalidInput)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	tokenKey := GenerateFromPattern("[a-zA-Z0-9]{50}")
	cols := []string{"email", "password_hash", "token_key"}
	placeholders := []string{"$1", "$2", "$3"}
	args := []any{email, hash, tokenKey}
	idx := 4

	isPrivileged := claims != nil && (claims.Role == "superuser" || claims.Role == "admin")
	for k, v := range extras {
		if !models.IsValidIdentifier(k) || k == "id" || k == "email" || k == "password" || k == "password_hash" || k == "token_key" || k == "created_at" || k == "updated_at" {
			continue
		}
		if k == "role" && !isPrivileged {
			continue
		}
		if k == "verified" && !isPrivileged {
			continue
		}
		if v == nil {
			continue
		}
		cols = append(cols, fmt.Sprintf(`"%s"`, k))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, v)
		idx++
	}

	query := fmt.Sprintf(`
		INSERT INTO users (%s)
		VALUES (%s)
		RETURNING id::text, email, password_hash, role, created_at, updated_at`,
		strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	var user models.User
	err = s.pool.QueryRow(ctx, query, args...).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailExists
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	token, err := s.GenerateToken(user)
	if err != nil {
		return nil, err
	}
	return &models.AuthResponse{Token: token, User: user}, nil
}

// Login verifies credentials and returns a signed JWT.
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest) (*models.AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var user models.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, COALESCE(password_hash, ''), role, created_at, updated_at
		FROM users
		WHERE email = $1`, email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	ok, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	token, err := s.GenerateToken(user)
	if err != nil {
		return nil, err
	}
	return &models.AuthResponse{Token: token, User: user}, nil
}

// GetUserByID fetches a user profile by ID.
func (s *AuthService) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, COALESCE(password_hash, ''), role, created_at, updated_at
		FROM users
		WHERE id = $1`, id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &user, nil
}

// GenerateToken signs an HS256 JWT for the given user.
func (s *AuthService) GenerateToken(user models.User) (string, error) {
	if len(s.jwtSecret) == 0 {
		return "", ErrMissingJWTSecret
	}

	now := time.Now()
	claims := AuthClaims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ParseToken validates a JWT string and returns its AuthClaims.
func (s *AuthService) ParseToken(tokenString string) (*AuthClaims, error) {
	if len(s.jwtSecret) == 0 {
		return nil, ErrMissingJWTSecret
	}

	token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || claims.Scope != "" || claims.ExpiresAt == nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ListUsers returns paginated user records from users while enforcing optional row-level API rule SQL.
func (s *AuthService) ListUsers(ctx context.Context, page, perPage int, search string, ruleSQL string, ruleArgs []any, sortParam ...string) ([]models.Record, int64, error) {
	offset := (page - 1) * perPage
	q := strings.TrimSpace(search)

	clauses := make([]string, 0, 2)
	args := make([]any, 0, len(ruleArgs)+3)
	idx := 1

	if ruleSQL != "" {
		clauses = append(clauses, "("+ruleSQL+")")
		args = append(args, ruleArgs...)
		idx += len(ruleArgs)
	}
	if q != "" {
		clauses = append(clauses, fmt.Sprintf("email ILIKE $%d", idx))
		args = append(args, "%"+q+"%")
		idx++
	}

	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = " WHERE " + strings.Join(clauses, " AND ")
	}

	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users"+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), perPage, offset)
	orderSQL := `"id" DESC`
	if len(sortParam) > 0 {
		rawSort := strings.TrimSpace(sortParam[0])
		if rawSort != "" {
			dir := "ASC"
			col := rawSort
			if strings.HasPrefix(col, "-") {
				dir = "DESC"
				col = strings.TrimPrefix(col, "-")
			} else if strings.HasPrefix(col, "+") {
				col = strings.TrimPrefix(col, "+")
			}
			if models.IsValidIdentifier(col) && col != "password_hash" && col != "token_key" {
				orderSQL = fmt.Sprintf(`"%s" %s`, col, dir)
			}
		}
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf("SELECT * FROM users%s ORDER BY %s LIMIT $%d OFFSET $%d", whereSQL, orderSQL, idx, idx+1), listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	items := make([]models.Record, 0)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, 0, err
		}
		rec := make(models.Record, len(descs))
		for i, fd := range descs {
			col := string(fd.Name)
			if col == "password_hash" || col == "token_key" {
				continue
			}
			rec[col] = vals[i]
		}
		items = append(items, rec)
	}
	return items, total, rows.Err()
}

func (s *AuthService) userExists(ctx context.Context, id string) bool {
	var one int
	return s.pool.QueryRow(ctx, `SELECT 1 FROM users WHERE id::text = $1`, id).Scan(&one) == nil
}

// GetUserRecord fetches a single user record from users while enforcing optional row-level API rule SQL (starting at $2).
func (s *AuthService) GetUserRecord(ctx context.Context, id string, ruleSQL string, ruleArgs []any) (models.Record, error) {
	query := `SELECT * FROM users WHERE id::text = $1`
	args := []any{id}
	if ruleSQL != "" {
		query += " AND (" + ruleSQL + ")"
		args = append(args, ruleArgs...)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if ruleSQL != "" && s.userExists(ctx, id) {
			return nil, ErrForbidden
		}
		return nil, ErrRecordNotFound
	}
	descs := rows.FieldDescriptions()
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	rec := make(models.Record, len(descs))
	for i, fd := range descs {
		col := string(fd.Name)
		if col == "password_hash" || col == "token_key" {
			continue
		}
		rec[col] = vals[i]
	}
	return rec, nil
}

// DeleteUser removes a user account by ID while enforcing optional row-level API rule SQL (starting at $2).
func (s *AuthService) DeleteUser(ctx context.Context, id string, ruleSQL string, ruleArgs []any) error {
	query := `DELETE FROM users WHERE id::text = $1`
	args := []any{id}
	if ruleSQL != "" {
		query += " AND (" + ruleSQL + ")"
		args = append(args, ruleArgs...)
	}
	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if ruleSQL != "" && s.userExists(ctx, id) {
			return ErrForbidden
		}
		return ErrRecordNotFound
	}
	_, _ = s.pool.Exec(ctx, `DELETE FROM _oauth_accounts WHERE user_id = $1`, id)
	return nil
}

// UpdateUser updates a user's email, role, optional password, and custom users columns while enforcing the users "update" API rule.
func (s *AuthService) UpdateUser(ctx context.Context, id string, email, role, newPassword string, custom map[string]any, usersCol *models.Collection, claims *AuthClaims) (models.Record, error) {
	setClauses := []string{"updated_at = NOW()"}
	args := []any{id}
	argIdx := 2

	isPrivileged := claims != nil && (claims.Role == "superuser" || claims.Role == "admin")

	if strings.TrimSpace(email) != "" {
		email = strings.ToLower(strings.TrimSpace(email))
		if !strings.Contains(email, "@") || strings.ContainsAny(email, "\r\n") {
			return nil, fmt.Errorf("%w: valid email is required", ErrInvalidInput)
		}
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, email)
		argIdx++
		if !isPrivileged {
			setClauses = append(setClauses, "verified = FALSE")
		}
	}

	if strings.TrimSpace(role) != "" {
		// Prevent non-admin/non-superuser from escalating their own role.
		if !isPrivileged {
			role = ""
		}
	}
	if strings.TrimSpace(role) != "" {
		setClauses = append(setClauses, fmt.Sprintf("role = $%d", argIdx))
		args = append(args, strings.TrimSpace(role))
		argIdx++
	}

	if newPassword != "" {
		if len(newPassword) < 8 {
			return nil, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
		}
		hash, hashErr := HashPassword(newPassword)
		if hashErr != nil {
			return nil, hashErr
		}
		setClauses = append(setClauses, fmt.Sprintf("password_hash = $%d", argIdx))
		args = append(args, hash)
		argIdx++
	}

	fieldByName := make(map[string]models.FieldDef)
	if usersCol != nil {
		for _, f := range usersCol.Fields {
			fieldByName[f.Name] = f
		}
	}
	for k, v := range custom {
		if !models.IsValidIdentifier(k) || k == "id" || k == "email" || k == "role" || k == "password" || k == "password_hash" || k == "token_key" || k == "created_at" || k == "updated_at" {
			continue
		}
		if k == "verified" && !isPrivileged {
			continue
		}
		if f, ok := fieldByName[k]; ok && (f.Type == "relation" || f.Type == "select" || f.Type == "file") && f.MaxSelect > 1 {
			if strVal, isStr := v.(string); isStr {
				if strings.TrimSpace(strVal) == "" {
					v = []string{}
				} else {
					v = []string{strVal}
				}
			}
		}
		setClauses = append(setClauses, fmt.Sprintf(`"%s" = $%d`, k, argIdx))
		args = append(args, v)
		argIdx++
	}

	ruleSQL, ruleArgs, _, ruleErr := CompileAPIRule(usersCol, "update", claims, custom, argIdx)
	if ruleErr != nil {
		return nil, ruleErr
	}

	wherePart := "id = $1"
	if ruleSQL != "" {
		wherePart += " AND (" + ruleSQL + ")"
		args = append(args, ruleArgs...)
	}

	query := fmt.Sprintf(`UPDATE users SET %s WHERE %s RETURNING *`, strings.Join(setClauses, ", "), wherePart)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailExists
		}
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, ErrEmailExists
			}
			return nil, err
		}
		if ruleSQL != "" && s.userExists(ctx, id) {
			return nil, ErrForbidden
		}
		return nil, ErrRecordNotFound
	}
	descs := rows.FieldDescriptions()
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	rec := make(models.Record, len(descs))
	for i, fd := range descs {
		col := string(fd.Name)
		if col == "password_hash" || col == "token_key" {
			continue
		}
		rec[col] = vals[i]
	}
	return rec, nil
}

// GenerateCollectionToken signs a JWT for a record in any Auth collection (e.g. "users", "staff", "students").
func (s *AuthService) GenerateCollectionToken(collection, recordID, email, role string) (string, error) {
	if len(s.jwtSecret) == 0 {
		return "", ErrMissingJWTSecret
	}
	if role == "" {
		role = "user"
	}
	now := time.Now()
	claims := AuthClaims{
		RecordID:   recordID,
		Collection: collection,
		Email:      email,
		Role:       role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   recordID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

// GenerateFileToken mints a short-lived (2-minute) signed JWT scoped strictly for protected file access.
func (s *AuthService) GenerateFileToken(claims *AuthClaims) (string, error) {
	if claims == nil {
		return "", ErrInvalidToken
	}
	now := time.Now()
	exp := now.Add(2 * time.Minute)
	sub := claims.RecordID
	if sub == "" {
		sub = claims.UserID
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   sub,
		"email": claims.Email,
		"role":  claims.Role,
		"scope": "file",
		"iat":   now.Unix(),
		"exp":   exp.Unix(),
	})
	return tok.SignedString(s.jwtSecret)
}

// ValidateFileToken verifies a short-lived file access token minted by GenerateFileToken.
func (s *AuthService) ValidateFileToken(tokenStr string) (*AuthClaims, error) {
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || fmt.Sprint(mc["scope"]) != "file" {
		return nil, ErrInvalidToken
	}
	sub := fmt.Sprint(mc["sub"])
	return &AuthClaims{
		UserID:   sub,
		RecordID: sub,
		Email:    fmt.Sprint(mc["email"]),
		Role:     fmt.Sprint(mc["role"]),
	}, nil
}
