package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/models"
)

// SuperuserService manages _superusers admin accounts separately from app _users.
type SuperuserService struct {
	pool *pgxpool.Pool
	auth *AuthService
}

// NewSuperuserService constructs a SuperuserService.
func NewSuperuserService(pool *pgxpool.Pool, authSvc *AuthService) *SuperuserService {
	return &SuperuserService{pool: pool, auth: authSvc}
}

// Setup creates a new superuser account (allowed when no superusers exist or by existing superuser).
func (s *SuperuserService) Setup(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") || len(req.Password) < 8 {
		return nil, fmt.Errorf("%w: valid email and password (min 8 chars) required", ErrInvalidInput)
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	var u models.User
	u.Role = "superuser"
	err = s.pool.QueryRow(ctx, `
		INSERT INTO _superusers (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, created_at, updated_at`,
		email, hash,
	).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailExists
		}
		return nil, fmt.Errorf("create superuser: %w", err)
	}

	token, err := s.auth.GenerateToken(u)
	if err != nil {
		return nil, err
	}
	return &models.AuthResponse{Token: token, User: u}, nil
}

// Login authenticates a superuser and issues a JWT with role "superuser".
func (s *SuperuserService) Login(ctx context.Context, req models.LoginRequest) (*models.AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var u models.User
	u.Role = "superuser"
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, created_at, updated_at
		FROM _superusers WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	ok, err := VerifyPassword(req.Password, u.PasswordHash)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	token, err := s.auth.GenerateToken(u)
	if err != nil {
		return nil, err
	}
	return &models.AuthResponse{Token: token, User: u}, nil
}

// HasSuperuser reports whether at least one superuser account has been initialized.
func (s *SuperuserService) HasSuperuser(ctx context.Context) (bool, error) {
	var count int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _superusers`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
