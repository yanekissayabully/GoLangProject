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

type UserRepository struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, email, password_hash, role, first_name, last_name, phone, is_email_verified, onboarding_status, profile_photo_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	// Default onboarding status
	if user.OnboardingStatus == "" {
		user.OnboardingStatus = models.OnboardingCreated
	}

	err := r.db.Pool.QueryRow(ctx, query,
		user.ID,
		strings.ToLower(user.Email),
		user.PasswordHash,
		user.Role,
		user.FirstName,
		user.LastName,
		user.Phone,
		user.IsEmailVerified,
		user.OnboardingStatus,
		user.ProfilePhotoURL,
	).Scan(&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "email") {
			return models.ErrEmailTaken
		}
		return err
	}

	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, role, first_name, last_name, phone, is_email_verified, onboarding_status, profile_photo_url, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &models.User{}
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.IsEmailVerified,
		&user.OnboardingStatus,
		&user.ProfilePhotoURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, role, first_name, last_name, phone, is_email_verified, onboarding_status, profile_photo_url, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	user := &models.User{}
	err := r.db.Pool.QueryRow(ctx, query, strings.ToLower(email)).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.IsEmailVerified,
		&user.OnboardingStatus,
		&user.ProfilePhotoURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users
		SET email = $2, password_hash = $3, role = $4, first_name = $5, last_name = $6, phone = $7, is_email_verified = $8, onboarding_status = $9, profile_photo_url = $10
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query,
		user.ID,
		strings.ToLower(user.Email),
		user.PasswordHash,
		user.Role,
		user.FirstName,
		user.LastName,
		user.Phone,
		user.IsEmailVerified,
		user.OnboardingStatus,
		user.ProfilePhotoURL,
	).Scan(&user.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrUserNotFound
		}
		return err
	}

	return nil
}

func (r *UserRepository) VerifyEmail(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET is_email_verified = TRUE WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, userID)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	query := `UPDATE users SET password_hash = $2 WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, userID, passwordHash)
	return err
}

func (r *UserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role models.Role) error {
	query := `UPDATE users SET role = $2, onboarding_status = $3, updated_at = NOW() WHERE id = $1`
	result, err := r.db.Pool.Exec(ctx, query, userID, role, models.OnboardingRoleSelected)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return models.ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) UpdateOnboardingStatus(ctx context.Context, userID uuid.UUID, status models.OnboardingStatus) error {
	query := `UPDATE users SET onboarding_status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.Pool.Exec(ctx, query, userID, status)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return models.ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) UpdateProfilePhoto(ctx context.Context, userID uuid.UUID, photoURL string) error {
	query := `UPDATE users SET profile_photo_url = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.Pool.Exec(ctx, query, userID, photoURL)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return models.ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`
	var exists bool
	err := r.db.Pool.QueryRow(ctx, query, strings.ToLower(email)).Scan(&exists)
	return exists, err
}

// OTP Rate Limiting

func (r *UserRepository) GetOTPSendCount(ctx context.Context, email string, since time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM otp_rate_limits WHERE email = $1 AND created_at > $2`
	var count int
	err := r.db.Pool.QueryRow(ctx, query, strings.ToLower(email), since).Scan(&count)
	return count, err
}

func (r *UserRepository) RecordOTPSend(ctx context.Context, email, ipAddress string) error {
	query := `INSERT INTO otp_rate_limits (email, ip_address) VALUES ($1, $2)`
	_, err := r.db.Pool.Exec(ctx, query, strings.ToLower(email), ipAddress)
	return err
}

func (r *UserRepository) CleanupOldRateLimits(ctx context.Context, before time.Time) error {
	query := `DELETE FROM otp_rate_limits WHERE created_at < $1`
	_, err := r.db.Pool.Exec(ctx, query, before)
	return err
}

// GetLastSeenActionsAt returns when the user last viewed their Today actions.
func (r *UserRepository) GetLastSeenActionsAt(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	var t time.Time
	err := r.db.Pool.QueryRow(ctx, `SELECT last_seen_actions_at FROM users WHERE id = $1`, userID).Scan(&t)
	return t, err
}

// UpdateLastSeenActionsAt sets last_seen_actions_at to now.
func (r *UserRepository) UpdateLastSeenActionsAt(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE users SET last_seen_actions_at = NOW() WHERE id = $1`, userID)
	return err
}
