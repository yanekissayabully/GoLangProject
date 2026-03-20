package repository

import (
	"context"
	"errors"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DocumentRepository struct {
	db *database.DB
}

func NewDocumentRepository(db *database.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) Create(ctx context.Context, doc *models.Document) error {
	query := `
		INSERT INTO documents (id, user_id, type, file_name, file_path, file_size, mime_type, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at
	`

	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}

	err := r.db.Pool.QueryRow(ctx, query,
		doc.ID,
		doc.UserID,
		doc.Type,
		doc.FileName,
		doc.FilePath,
		doc.FileSize,
		doc.MimeType,
		doc.Status,
	).Scan(&doc.CreatedAt, &doc.UpdatedAt)

	return err
}

func (r *DocumentRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Document, error) {
	query := `
		SELECT id, user_id, type, file_name, file_path, file_size, mime_type, status, created_at, updated_at
		FROM documents
		WHERE id = $1
	`

	doc := &models.Document{}
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&doc.ID,
		&doc.UserID,
		&doc.Type,
		&doc.FileName,
		&doc.FilePath,
		&doc.FileSize,
		&doc.MimeType,
		&doc.Status,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return doc, nil
}

func (r *DocumentRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Document, error) {
	query := `
		SELECT id, user_id, type, file_name, file_path, file_size, mime_type, status, created_at, updated_at
		FROM documents
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []*models.Document
	for rows.Next() {
		doc := &models.Document{}
		err := rows.Scan(
			&doc.ID,
			&doc.UserID,
			&doc.Type,
			&doc.FileName,
			&doc.FilePath,
			&doc.FileSize,
			&doc.MimeType,
			&doc.Status,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}

	return docs, rows.Err()
}

func (r *DocumentRepository) GetByUserIDAndType(ctx context.Context, userID uuid.UUID, docType models.DocumentType) (*models.Document, error) {
	query := `
		SELECT id, user_id, type, file_name, file_path, file_size, mime_type, status, created_at, updated_at
		FROM documents
		WHERE user_id = $1 AND type = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	doc := &models.Document{}
	err := r.db.Pool.QueryRow(ctx, query, userID, docType).Scan(
		&doc.ID,
		&doc.UserID,
		&doc.Type,
		&doc.FileName,
		&doc.FilePath,
		&doc.FileSize,
		&doc.MimeType,
		&doc.Status,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return doc, nil
}

func (r *DocumentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.DocumentStatus) error {
	query := `UPDATE documents SET status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.Pool.Exec(ctx, query, id, status)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return models.ErrUserNotFound // reusing error for simplicity
	}
	return nil
}

func (r *DocumentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM documents WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, id)
	return err
}

func (r *DocumentRepository) DeleteByUserIDAndType(ctx context.Context, userID uuid.UUID, docType models.DocumentType) error {
	query := `DELETE FROM documents WHERE user_id = $1 AND type = $2`
	_, err := r.db.Pool.Exec(ctx, query, userID, docType)
	return err
}

// HasRequiredDocuments checks if driver has uploaded required documents
func (r *DocumentRepository) HasRequiredDocuments(ctx context.Context, userID uuid.UUID) (bool, error) {
	query := `
		SELECT COUNT(DISTINCT type)
		FROM documents
		WHERE user_id = $1 AND type IN ($2, $3)
	`
	var count int
	err := r.db.Pool.QueryRow(ctx, query, userID, models.DocumentDriversLicense, models.DocumentRegistration).Scan(&count)
	if err != nil {
		return false, err
	}
	return count >= 2, nil
}
