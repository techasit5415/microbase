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
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
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
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || len(req.Password) < 8 {
		return nil, fmt.Errorf("%w: valid email and password (min 8 chars) required", ErrInvalidInput)
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	var user models.User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO _users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, role, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
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
		FROM _users
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
func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, COALESCE(password_hash, ''), role, created_at, updated_at
		FROM _users
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
	if !ok {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ListUsers returns paginated user records from _users while enforcing optional row-level API rule SQL.
func (s *AuthService) ListUsers(ctx context.Context, page, perPage int, search string, ruleSQL string, ruleArgs []any) ([]models.Record, int64, error) {
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
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM _users"+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), perPage, offset)
	rows, err := s.pool.Query(ctx, fmt.Sprintf("SELECT * FROM _users%s ORDER BY id DESC LIMIT $%d OFFSET $%d", whereSQL, idx, idx+1), listArgs...)
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
			if col == "password_hash" {
				continue
			}
			rec[col] = vals[i]
		}
		items = append(items, rec)
	}
	return items, total, rows.Err()
}

func (s *AuthService) userExists(ctx context.Context, id int64) bool {
	var one int
	return s.pool.QueryRow(ctx, `SELECT 1 FROM _users WHERE id = $1`, id).Scan(&one) == nil
}

// GetUserRecord fetches a single user record from _users while enforcing optional row-level API rule SQL (starting at $2).
func (s *AuthService) GetUserRecord(ctx context.Context, id int64, ruleSQL string, ruleArgs []any) (models.Record, error) {
	query := `SELECT * FROM _users WHERE id = $1`
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
		if col == "password_hash" {
			continue
		}
		rec[col] = vals[i]
	}
	return rec, nil
}

// DeleteUser removes a user account by ID while enforcing optional row-level API rule SQL (starting at $2).
func (s *AuthService) DeleteUser(ctx context.Context, id int64, ruleSQL string, ruleArgs []any) error {
	query := `DELETE FROM _users WHERE id = $1`
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
	return nil
}

// UpdateUser updates a user's email, role, optional password, and custom _users columns while enforcing the users "update" API rule.
func (s *AuthService) UpdateUser(ctx context.Context, id int64, email, role, newPassword string, custom map[string]any, usersCol *models.Collection, claims *AuthClaims) (models.Record, error) {
	setClauses := []string{"updated_at = NOW()"}
	args := []any{id}
	argIdx := 2

	if strings.TrimSpace(email) != "" {
		email = strings.ToLower(strings.TrimSpace(email))
		if !strings.Contains(email, "@") {
			return nil, fmt.Errorf("%w: valid email is required", ErrInvalidInput)
		}
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, email)
		argIdx++
	}

	if strings.TrimSpace(role) != "" {
		// Prevent non-admin/non-superuser from escalating their own role.
		if claims == nil || (claims.Role != "superuser" && claims.Role != "admin") {
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

	for k, v := range custom {
		if !models.IsValidIdentifier(k) || k == "id" || k == "email" || k == "role" || k == "password" || k == "password_hash" || k == "created_at" || k == "updated_at" {
			continue
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

	query := fmt.Sprintf(`UPDATE _users SET %s WHERE %s RETURNING *`, strings.Join(setClauses, ", "), wherePart)
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
		if col == "password_hash" {
			continue
		}
		rec[col] = vals[i]
	}
	return rec, nil
}
