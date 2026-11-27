package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
)

const (
	setupOTPKey = "microbase:setup_otp"
	setupOTPTTL = 5 * time.Minute
)

var (
	ErrSetupLocked     = errors.New("superuser setup is locked because a superuser already exists")
	ErrInvalidSetupOTP = errors.New("invalid or expired 8-digit setup OTP (restart server for a new OTP)")
	ErrLastSuperuser   = errors.New("cannot delete the last remaining superuser")
)

// SuperuserSetupRequest is the payload accepted by POST /api/superusers/setup.
type SuperuserSetupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OTP      string `json:"otp"`
}

// SuperuserService manages _superusers admin accounts and first-run OTP setup.
type SuperuserService struct {
	pool  *pgxpool.Pool
	auth  *AuthService
	cache cache.CacheStore
}

func NewSuperuserService(pool *pgxpool.Pool, authSvc *AuthService, c cache.CacheStore) *SuperuserService {
	return &SuperuserService{pool: pool, auth: authSvc, cache: c}
}

// HasSuperuser reports whether at least one superuser account exists.
func (s *SuperuserService) HasSuperuser(ctx context.Context) (bool, error) {
	var count int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _superusers`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// GenerateSetupOTP creates a cryptographically random 8-digit OTP valid for 5 minutes.
func GenerateSetupOTP(ctx context.Context, c cache.CacheStore) (string, error) {
	max := big.NewInt(100000000) // 0..99999999
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	otp := fmt.Sprintf("%08d", n.Int64())
	if err := c.Set(ctx, setupOTPKey, otp, setupOTPTTL); err != nil {
		return "", err
	}
	return otp, nil
}

// Setup creates the first superuser account after verifying the 8-digit 5m OTP and locks setup thereafter.
func (s *SuperuserService) Setup(ctx context.Context, req SuperuserSetupRequest) (*models.AuthResponse, error) {
	has, err := s.HasSuperuser(ctx)
	if err != nil {
		return nil, err
	}
	if has {
		return nil, ErrSetupLocked
	}

	expectedOTP, err := s.cache.Get(ctx, setupOTPKey)
	if err != nil || expectedOTP == "" {
		return nil, ErrInvalidSetupOTP
	}
	providedOTP := strings.TrimSpace(req.OTP)
	if len(providedOTP) != len(expectedOTP) || subtle.ConstantTimeCompare([]byte(providedOTP), []byte(expectedOTP)) != 1 {
		const maxSetupOTPAttempts = 5
		if attempts, incrErr := s.cache.Incr(ctx, setupOTPKey+":fail", setupOTPTTL); incrErr == nil && attempts >= maxSetupOTPAttempts {
			_ = s.cache.Delete(ctx, setupOTPKey)
			_ = s.cache.Delete(ctx, setupOTPKey+":fail")
		}
		return nil, ErrInvalidSetupOTP
	}

	u, err := s.Create(ctx, models.RegisterRequest{Email: req.Email, Password: req.Password})
	if err != nil {
		return nil, err
	}

	_ = s.cache.Delete(ctx, setupOTPKey)
	_ = s.cache.Delete(ctx, setupOTPKey+":fail")

	token, err := s.auth.GenerateToken(*u)
	if err != nil {
		return nil, err
	}
	return &models.AuthResponse{Token: token, User: *u}, nil
}

// Create inserts a new superuser account with an Argon2id password hash.
func (s *SuperuserService) Create(ctx context.Context, req models.RegisterRequest) (*models.User, error) {
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
	return &u, nil
}

// Delete removes a superuser by ID unless it is the last remaining superuser.
func (s *SuperuserService) Delete(ctx context.Context, id int64) error {
	var count int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _superusers`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastSuperuser
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM _superusers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
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

// Update updates a superuser's email and optionally resets their password.
func (s *SuperuserService) Update(ctx context.Context, id int64, email, newPassword string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return nil, fmt.Errorf("%w: valid email is required", ErrInvalidInput)
	}

	var u models.User
	u.Role = "superuser"
	var err error
	if newPassword != "" {
		if len(newPassword) < 8 {
			return nil, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
		}
		hash, hashErr := HashPassword(newPassword)
		if hashErr != nil {
			return nil, hashErr
		}
		err = s.pool.QueryRow(ctx, `
			UPDATE _superusers
			SET email = $2, password_hash = $3, updated_at = NOW()
			WHERE id = $1
			RETURNING id, email, created_at, updated_at`,
			id, email, hash,
		).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	} else {
		err = s.pool.QueryRow(ctx, `
			UPDATE _superusers
			SET email = $2, updated_at = NOW()
			WHERE id = $1
			RETURNING id, email, created_at, updated_at`,
			id, email,
		).Scan(&u.ID, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailExists
		}
		return nil, err
	}
	return &u, nil
}
