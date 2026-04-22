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

type UserRepository struct{ db *database.DB }

func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	if user.OnboardingStatus == "" {
		user.OnboardingStatus = models.OnboardingCreated
	}

	query := `
		INSERT INTO users (id, email, password_hash, role, first_name, last_name, phone, is_email_verified, onboarding_status, profile_photo_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`
	err := r.db.Pool.QueryRow(ctx, query,
		user.ID, strings.ToLower(user.Email), user.PasswordHash, user.Role,
		user.FirstName, user.LastName, user.Phone,
		user.IsEmailVerified, user.OnboardingStatus, user.ProfilePhotoURL,
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
		SELECT id, email, password_hash, role, first_name, last_name, phone,
		       is_email_verified, onboarding_status, profile_photo_url, created_at, updated_at
		FROM users WHERE id = $1
	`
	u := &models.User{}
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role,
		&u.FirstName, &u.LastName, &u.Phone,
		&u.IsEmailVerified, &u.OnboardingStatus, &u.ProfilePhotoURL,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, role, first_name, last_name, phone,
		       is_email_verified, onboarding_status, profile_photo_url, created_at, updated_at
		FROM users WHERE email = $1
	`
	u := &models.User{}
	err := r.db.Pool.QueryRow(ctx, query, strings.ToLower(email)).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role,
		&u.FirstName, &u.LastName, &u.Phone,
		&u.IsEmailVerified, &u.OnboardingStatus, &u.ProfilePhotoURL,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`,
		strings.ToLower(email),
	).Scan(&exists)
	return exists, err
}

// OTP rate-limit helpers

func (r *UserRepository) GetOTPSendCount(ctx context.Context, key string, since time.Time) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM otp_rate_limits WHERE email = $1 AND created_at > $2`,
		strings.ToLower(key), since,
	).Scan(&count)
	return count, err
}

func (r *UserRepository) RecordOTPSend(ctx context.Context, key, ip string) error {
	_, err := r.db.Pool.Exec(ctx,
		`INSERT INTO otp_rate_limits (email, ip_address) VALUES ($1, $2)`,
		strings.ToLower(key), ip,
	)
	return err
}
