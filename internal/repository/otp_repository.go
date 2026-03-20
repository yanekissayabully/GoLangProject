package repository

import (
	"context"
	"errors"
	"time"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type OTPRepository struct {
	db *database.DB
}

func NewOTPRepository(db *database.DB) *OTPRepository {
	return &OTPRepository{db: db}
}

func (r *OTPRepository) Create(ctx context.Context, otp *models.EmailOTP) error {
	query := `
		INSERT INTO email_otps (id, user_id, purpose, code_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`

	if otp.ID == uuid.Nil {
		otp.ID = uuid.New()
	}

	err := r.db.Pool.QueryRow(ctx, query,
		otp.ID,
		otp.UserID,
		otp.Purpose,
		otp.CodeHash,
		otp.ExpiresAt,
	).Scan(&otp.CreatedAt)

	return err
}

func (r *OTPRepository) GetLatestActiveByUserAndPurpose(ctx context.Context, userID uuid.UUID, purpose models.OTPPurpose) (*models.EmailOTP, error) {
	query := `
		SELECT id, user_id, purpose, code_hash, expires_at, created_at, consumed_at
		FROM email_otps
		WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`

	otp := &models.EmailOTP{}
	err := r.db.Pool.QueryRow(ctx, query, userID, purpose).Scan(
		&otp.ID,
		&otp.UserID,
		&otp.Purpose,
		&otp.CodeHash,
		&otp.ExpiresAt,
		&otp.CreatedAt,
		&otp.ConsumedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No active OTP found
		}
		return nil, err
	}

	return otp, nil
}

func (r *OTPRepository) MarkConsumed(ctx context.Context, otpID uuid.UUID) error {
	query := `UPDATE email_otps SET consumed_at = NOW() WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, otpID)
	return err
}

func (r *OTPRepository) InvalidateAllForUser(ctx context.Context, userID uuid.UUID, purpose models.OTPPurpose) error {
	query := `UPDATE email_otps SET consumed_at = NOW() WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL`
	_, err := r.db.Pool.Exec(ctx, query, userID, purpose)
	return err
}

func (r *OTPRepository) CleanupExpired(ctx context.Context) error {
	// Delete OTPs that are both expired and older than 24 hours
	query := `DELETE FROM email_otps WHERE expires_at < NOW() AND created_at < NOW() - INTERVAL '24 hours'`
	_, err := r.db.Pool.Exec(ctx, query)
	return err
}

// GetByUserIDPurposeAndHash finds OTP for verification
func (r *OTPRepository) GetByUserIDPurposeAndHash(ctx context.Context, userID uuid.UUID, purpose models.OTPPurpose, codeHash string) (*models.EmailOTP, error) {
	query := `
		SELECT id, user_id, purpose, code_hash, expires_at, created_at, consumed_at
		FROM email_otps
		WHERE user_id = $1 AND purpose = $2 AND code_hash = $3 AND consumed_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	otp := &models.EmailOTP{}
	err := r.db.Pool.QueryRow(ctx, query, userID, purpose, codeHash).Scan(
		&otp.ID,
		&otp.UserID,
		&otp.Purpose,
		&otp.CodeHash,
		&otp.ExpiresAt,
		&otp.CreatedAt,
		&otp.ConsumedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return otp, nil
}

// CountRecentOTPs counts how many OTPs have been created for a user in the given duration
func (r *OTPRepository) CountRecentOTPs(ctx context.Context, userID uuid.UUID, purpose models.OTPPurpose, since time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM email_otps WHERE user_id = $1 AND purpose = $2 AND created_at > $3`
	var count int
	err := r.db.Pool.QueryRow(ctx, query, userID, purpose, since).Scan(&count)
	return count, err
}
