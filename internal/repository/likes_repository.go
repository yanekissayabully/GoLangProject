package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/drivebai/backend/internal/database"
)

type LikesRepository struct {
	db *database.DB
}

func NewLikesRepository(db *database.DB) *LikesRepository {
	return &LikesRepository{db: db}
}

// AddLike adds a like for a user on a listing
func (r *LikesRepository) AddLike(ctx context.Context, userID, listingID uuid.UUID) error {
	query := `
		INSERT INTO listing_likes (user_id, listing_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, listing_id) DO NOTHING
	`
	_, err := r.db.Pool.Exec(ctx, query, userID, listingID)
	return err
}

// RemoveLike removes a like for a user on a listing
func (r *LikesRepository) RemoveLike(ctx context.Context, userID, listingID uuid.UUID) error {
	query := `DELETE FROM listing_likes WHERE user_id = $1 AND listing_id = $2`
	_, err := r.db.Pool.Exec(ctx, query, userID, listingID)
	return err
}

// GetLikedListingIDs returns all listing IDs that a user has liked
func (r *LikesRepository) GetLikedListingIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT listing_id FROM listing_likes WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := r.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

// IsLiked checks if a user has liked a specific listing
func (r *LikesRepository) IsLiked(ctx context.Context, userID, listingID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM listing_likes WHERE user_id = $1 AND listing_id = $2)`

	var exists bool
	err := r.db.Pool.QueryRow(ctx, query, userID, listingID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return exists, nil
}

// GetLikeCount returns the number of likes for a listing
func (r *LikesRepository) GetLikeCount(ctx context.Context, listingID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM listing_likes WHERE listing_id = $1`

	var count int
	err := r.db.Pool.QueryRow(ctx, query, listingID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
