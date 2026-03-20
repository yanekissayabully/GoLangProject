package repository

import (
	"context"
	"errors"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TokenRepository struct {
	db *database.DB
}

func NewTokenRepository(db *database.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

func (r *TokenRepository) CreateRefreshToken(ctx context.Context, token *models.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`

	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}

	err := r.db.Pool.QueryRow(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
	).Scan(&token.CreatedAt)

	return err
}

func (r *TokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	token := &models.RefreshToken{}
	err := r.db.Pool.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.RevokedAt,
		&token.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return token, nil
}

func (r *TokenRepository) RevokeToken(ctx context.Context, tokenID uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked_at = NOW() WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, tokenID)
	return err
}

func (r *TokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.db.Pool.Exec(ctx, query, userID)
	return err
}

func (r *TokenRepository) CleanupExpired(ctx context.Context) error {
	// Delete tokens that are both expired/revoked and older than 7 days
	query := `
		DELETE FROM refresh_tokens
		WHERE (expires_at < NOW() OR revoked_at IS NOT NULL)
		AND created_at < NOW() - INTERVAL '7 days'
	`
	_, err := r.db.Pool.Exec(ctx, query)
	return err
}

func (r *TokenRepository) GetActiveTokenCountForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()`
	var count int
	err := r.db.Pool.QueryRow(ctx, query, userID).Scan(&count)
	return count, err
}

// Password Reset Token methods

func (r *TokenRepository) CreatePasswordResetToken(ctx context.Context, token *models.PasswordResetToken) error {
	query := `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`

	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}

	err := r.db.Pool.QueryRow(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
	).Scan(&token.CreatedAt)

	return err
}

func (r *TokenRepository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE token_hash = $1
	`

	token := &models.PasswordResetToken{}
	err := r.db.Pool.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.UsedAt,
		&token.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return token, nil
}

func (r *TokenRepository) MarkPasswordResetTokenUsed(ctx context.Context, tokenID uuid.UUID) error {
	query := `UPDATE password_reset_tokens SET used_at = NOW() WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, tokenID)
	return err
}

func (r *TokenRepository) InvalidatePasswordResetTokensForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE password_reset_tokens SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL`
	_, err := r.db.Pool.Exec(ctx, query, userID)
	return err
}
