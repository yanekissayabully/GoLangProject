package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type LoginOTPRepository struct {
	db *database.DB
}

func NewLoginOTPRepository(db *database.DB) *LoginOTPRepository {
	return &LoginOTPRepository{db: db}
}

// Create inserts a new login OTP record and returns it.
func (r *LoginOTPRepository) Create(
	ctx context.Context,
	email, codeHash string,
	expiresAt time.Time,
	ipAddress, userAgent string,
) (*models.LoginOTP, error) {
	var ip *string
	if ipAddress != "" {
		ip = &ipAddress
	}
	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}

	otp := &models.LoginOTP{
		ID:        uuid.New(),
		Email:     strings.ToLower(email),
		CodeHash:  codeHash,
		ExpiresAt: expiresAt,
		IPAddress: ip,
		UserAgent: ua,
	}

	query := `
		INSERT INTO login_otps (id, email, code_hash, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING attempts, created_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		otp.ID, otp.Email, otp.CodeHash, otp.ExpiresAt, ip, ua,
	).Scan(&otp.Attempts, &otp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return otp, nil
}

// GetLatestUnconsumed returns the most recently created unconsumed OTP for the email.
// Returns nil, nil when none exists.
func (r *LoginOTPRepository) GetLatestUnconsumed(ctx context.Context, email string) (*models.LoginOTP, error) {
	query := `
		SELECT id, email, code_hash, expires_at, attempts, consumed_at, ip_address, user_agent, created_at
		FROM login_otps
		WHERE email = $1 AND consumed_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`
	otp := &models.LoginOTP{}
	err := r.db.Pool.QueryRow(ctx, query, strings.ToLower(email)).Scan(
		&otp.ID,
		&otp.Email,
		&otp.CodeHash,
		&otp.ExpiresAt,
		&otp.Attempts,
		&otp.ConsumedAt,
		&otp.IPAddress,
		&otp.UserAgent,
		&otp.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return otp, nil
}

// IncrementAttempts increments the attempt counter for the given OTP.
// It also returns the updated attempts count so the caller can check the limit.
func (r *LoginOTPRepository) IncrementAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	var attempts int
	err := r.db.Pool.QueryRow(ctx,
		`UPDATE login_otps SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`,
		id,
	).Scan(&attempts)
	return attempts, err
}

// MarkConsumed marks the OTP as used so it cannot be replayed.
func (r *LoginOTPRepository) MarkConsumed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE login_otps SET consumed_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

// CleanupExpired deletes all OTP records older than the given cutoff.
func (r *LoginOTPRepository) CleanupExpired(ctx context.Context, before time.Time) error {
	_, err := r.db.Pool.Exec(ctx,
		`DELETE FROM login_otps WHERE expires_at < $1`,
		before,
	)
	return err
}

// CountRecentByEmail counts OTPs created for an email within the given window.
// Used for per-email rate limiting.
func (r *LoginOTPRepository) CountRecentByEmail(ctx context.Context, email string, since time.Time) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM login_otps WHERE email = $1 AND created_at > $2`,
		strings.ToLower(email), since,
	).Scan(&count)
	return count, err
}
