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

// ListUsers returns paginated users from _users with optional email search.
func (s *AuthService) ListUsers(ctx context.Context, page, perPage int, search string) ([]models.User, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	q := strings.TrimSpace(search)

	var total int64
	if q != "" {
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _users WHERE email ILIKE $1`, "%"+q+"%").Scan(&total); err != nil {
			return nil, 0, err
		}
	} else {
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _users`).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	var rows pgx.Rows
	var err error
	if q != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, email, COALESCE(password_hash, ''), role, created_at, updated_at
			FROM _users WHERE email ILIKE $1 ORDER BY id DESC LIMIT $2 OFFSET $3`,
			"%"+q+"%", perPage, offset)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, email, COALESCE(password_hash, ''), role, created_at, updated_at
			FROM _users ORDER BY id DESC LIMIT $1 OFFSET $2`,
			perPage, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// DeleteUser removes a user account by ID.
func (s *AuthService) DeleteUser(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}
