package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"drivebai/internal/database"
	"drivebai/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type LoginOTPRepository struct{ db *database.DB }

func NewLoginOTPRepository(db *database.DB) *LoginOTPRepository {
	return &LoginOTPRepository{db: db}
}

func (r *LoginOTPRepository) Create(
	ctx context.Context, email, codeHash string, expiresAt time.Time, ip, ua string,
) (*models.LoginOTP, error) {
	var ipP, uaP *string
	if ip != "" {
		ipP = &ip
	}
	if ua != "" {
		uaP = &ua
	}

	otp := &models.LoginOTP{
		ID: uuid.New(), Email: strings.ToLower(email),
		CodeHash: codeHash, ExpiresAt: expiresAt,
		IPAddress: ipP, UserAgent: uaP,
	}

	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO login_otps (id, email, code_hash, expires_at, ip_address, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING attempts, created_at`,
		otp.ID, otp.Email, otp.CodeHash, otp.ExpiresAt, ipP, uaP,
	).Scan(&otp.Attempts, &otp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return otp, nil
}

func (r *LoginOTPRepository) GetLatestUnconsumed(ctx context.Context, email string) (*models.LoginOTP, error) {
	otp := &models.LoginOTP{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, email, code_hash, expires_at, attempts, consumed_at, ip_address, user_agent, created_at
		 FROM login_otps
		 WHERE email = $1 AND consumed_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`,
		strings.ToLower(email),
	).Scan(&otp.ID, &otp.Email, &otp.CodeHash, &otp.ExpiresAt, &otp.Attempts,
		&otp.ConsumedAt, &otp.IPAddress, &otp.UserAgent, &otp.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return otp, nil
}

func (r *LoginOTPRepository) IncrementAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	var attempts int
	err := r.db.Pool.QueryRow(ctx,
		`UPDATE login_otps SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`, id,
	).Scan(&attempts)
	return attempts, err
}

func (r *LoginOTPRepository) MarkConsumed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE login_otps SET consumed_at = NOW() WHERE id = $1`, id)
	return err
}
