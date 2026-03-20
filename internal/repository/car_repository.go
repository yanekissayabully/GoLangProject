package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
)

type CarRepository struct {
	db *database.DB
}

func NewCarRepository(db *database.DB) *CarRepository {
	return &CarRepository{db: db}
}

// Create creates a new car listing
func (r *CarRepository) Create(ctx context.Context, car *models.Car) error {
	query := `
		INSERT INTO cars (
			id, owner_id, title, description,
			make, model, year, body_type, fuel_type, mileage,
			address, neighborhood, latitude, longitude, area, street, block, zip,
			is_for_rent, weekly_rent_price, is_for_sale, sale_price, currency,
			min_years_licensed, deposit_amount, insurance_coverage,
			status, is_paused, rented_weeks, total_earned,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23,
			$24, $25, $26,
			$27, $28, $29, $30,
			$31, $32
		)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		car.ID, car.OwnerID, car.Title, car.Description,
		car.Make, car.Model, car.Year, car.BodyType, car.FuelType, car.Mileage,
		car.Address, car.Neighborhood, car.Latitude, car.Longitude, car.Area, car.Street, car.Block, car.Zip,
		car.IsForRent, car.WeeklyRentPrice, car.IsForSale, car.SalePrice, car.Currency,
		car.MinYearsLicensed, car.DepositAmount, car.InsuranceCoverage,
		car.Status, car.IsPaused, car.RentedWeeks, car.TotalEarned,
		car.CreatedAt, car.UpdatedAt,
	)

	return err
}

// GetByID retrieves a car by its ID
func (r *CarRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Car, error) {
	query := `
		SELECT
			id, owner_id, title, description,
			make, model, year, body_type, fuel_type, mileage,
			address, neighborhood, latitude, longitude, area, street, block, zip,
			is_for_rent, weekly_rent_price, is_for_sale, sale_price, currency,
			min_years_licensed, deposit_amount, insurance_coverage,
			status, is_paused, rented_weeks, total_earned,
			created_at, updated_at
		FROM cars
		WHERE id = $1
	`

	var car models.Car
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&car.ID, &car.OwnerID, &car.Title, &car.Description,
		&car.Make, &car.Model, &car.Year, &car.BodyType, &car.FuelType, &car.Mileage,
		&car.Address, &car.Neighborhood, &car.Latitude, &car.Longitude, &car.Area, &car.Street, &car.Block, &car.Zip,
		&car.IsForRent, &car.WeeklyRentPrice, &car.IsForSale, &car.SalePrice, &car.Currency,
		&car.MinYearsLicensed, &car.DepositAmount, &car.InsuranceCoverage,
		&car.Status, &car.IsPaused, &car.RentedWeeks, &car.TotalEarned,
		&car.CreatedAt, &car.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &car, nil
}

// GetByOwnerID retrieves all cars for a specific owner
func (r *CarRepository) GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]*models.Car, error) {
	query := `
		SELECT
			id, owner_id, title, description,
			make, model, year, body_type, fuel_type, mileage,
			address, neighborhood, latitude, longitude, area, street, block, zip,
			is_for_rent, weekly_rent_price, is_for_sale, sale_price, currency,
			min_years_licensed, deposit_amount, insurance_coverage,
			status, is_paused, rented_weeks, total_earned,
			created_at, updated_at
		FROM cars
		WHERE owner_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cars []*models.Car
	for rows.Next() {
		var car models.Car
		err := rows.Scan(
			&car.ID, &car.OwnerID, &car.Title, &car.Description,
			&car.Make, &car.Model, &car.Year, &car.BodyType, &car.FuelType, &car.Mileage,
			&car.Address, &car.Neighborhood, &car.Latitude, &car.Longitude, &car.Area, &car.Street, &car.Block, &car.Zip,
			&car.IsForRent, &car.WeeklyRentPrice, &car.IsForSale, &car.SalePrice, &car.Currency,
			&car.MinYearsLicensed, &car.DepositAmount, &car.InsuranceCoverage,
			&car.Status, &car.IsPaused, &car.RentedWeeks, &car.TotalEarned,
			&car.CreatedAt, &car.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		cars = append(cars, &car)
	}

	return cars, nil
}

// Update updates a car listing
func (r *CarRepository) Update(ctx context.Context, car *models.Car) error {
	query := `
		UPDATE cars SET
			title = $2, description = $3,
			make = $4, model = $5, year = $6, body_type = $7, fuel_type = $8, mileage = $9,
			address = $10, neighborhood = $11, latitude = $12, longitude = $13,
			area = $14, street = $15, block = $16, zip = $17,
			is_for_rent = $18, weekly_rent_price = $19, is_for_sale = $20, sale_price = $21,
			min_years_licensed = $22, deposit_amount = $23, insurance_coverage = $24,
			status = $25, is_paused = $26
		WHERE id = $1
	`

	result, err := r.db.Pool.Exec(ctx, query,
		car.ID, car.Title, car.Description,
		car.Make, car.Model, car.Year, car.BodyType, car.FuelType, car.Mileage,
		car.Address, car.Neighborhood, car.Latitude, car.Longitude,
		car.Area, car.Street, car.Block, car.Zip,
		car.IsForRent, car.WeeklyRentPrice, car.IsForSale, car.SalePrice,
		car.MinYearsLicensed, car.DepositAmount, car.InsuranceCoverage,
		car.Status, car.IsPaused,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("car not found")
	}

	return nil
}

// Delete deletes a car listing
func (r *CarRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM cars WHERE id = $1`
	result, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("car not found")
	}

	return nil
}

// UpdateStatus updates only the status and is_paused fields
func (r *CarRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.CarListingStatus, isPaused bool) error {
	query := `UPDATE cars SET status = $2, is_paused = $3 WHERE id = $1`
	result, err := r.db.Pool.Exec(ctx, query, id, status, isPaused)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("car not found")
	}

	return nil
}

// GetAvailableListings retrieves all available car listings for drivers to browse
func (r *CarRepository) GetAvailableListings(ctx context.Context, status string, search string) ([]*models.Car, error) {
	query := `
		SELECT
			id, owner_id, title, description,
			make, model, year, body_type, fuel_type, mileage,
			address, neighborhood, latitude, longitude, area, street, block, zip,
			is_for_rent, weekly_rent_price, is_for_sale, sale_price, currency,
			min_years_licensed, deposit_amount, insurance_coverage,
			status, is_paused, rented_weeks, total_earned,
			created_at, updated_at
		FROM cars
		WHERE is_paused = false
	`

	args := []interface{}{}
	argIndex := 1

	// Filter by status if provided
	if status != "" && status != "all" {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	// Search by make, model, or title
	if search != "" {
		query += fmt.Sprintf(" AND (LOWER(make) LIKE $%d OR LOWER(model) LIKE $%d OR LOWER(title) LIKE $%d)", argIndex, argIndex, argIndex)
		args = append(args, "%"+strings.ToLower(search)+"%")
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cars []*models.Car
	for rows.Next() {
		var car models.Car
		err := rows.Scan(
			&car.ID, &car.OwnerID, &car.Title, &car.Description,
			&car.Make, &car.Model, &car.Year, &car.BodyType, &car.FuelType, &car.Mileage,
			&car.Address, &car.Neighborhood, &car.Latitude, &car.Longitude, &car.Area, &car.Street, &car.Block, &car.Zip,
			&car.IsForRent, &car.WeeklyRentPrice, &car.IsForSale, &car.SalePrice, &car.Currency,
			&car.MinYearsLicensed, &car.DepositAmount, &car.InsuranceCoverage,
			&car.Status, &car.IsPaused, &car.RentedWeeks, &car.TotalEarned,
			&car.CreatedAt, &car.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		cars = append(cars, &car)
	}

	return cars, nil
}

// UpdateLocation updates only the location fields for a car
func (r *CarRepository) UpdateLocation(ctx context.Context, id uuid.UUID, lat, lng float64, area, street, block, zip string) error {
	query := `
		UPDATE cars SET
			latitude = $2, longitude = $3,
			area = $4, street = $5, block = $6, zip = $7
		WHERE id = $1
	`
	result, err := r.db.Pool.Exec(ctx, query, id, lat, lng, area, street, block, zip)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("car not found")
	}
	return nil
}

// CarPhotoRepository handles car photo database operations
type CarPhotoRepository struct {
	db *database.DB
}

func NewCarPhotoRepository(db *database.DB) *CarPhotoRepository {
	return &CarPhotoRepository{db: db}
}

// Create creates a new car photo record
func (r *CarPhotoRepository) Create(ctx context.Context, photo *models.CarPhoto) error {
	query := `
		INSERT INTO car_photos (id, car_id, slot_type, file_path, file_url, file_size, mime_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		photo.ID, photo.CarID, photo.SlotType, photo.FilePath, photo.FileURL,
		photo.FileSize, photo.MimeType, photo.CreatedAt, photo.UpdatedAt,
	)

	return err
}

// GetByCarID retrieves all photos for a car
func (r *CarPhotoRepository) GetByCarID(ctx context.Context, carID uuid.UUID) ([]models.CarPhoto, error) {
	query := `
		SELECT id, car_id, slot_type, file_path, file_url, file_size, mime_type, created_at, updated_at
		FROM car_photos
		WHERE car_id = $1
		ORDER BY slot_type
	`

	rows, err := r.db.Pool.Query(ctx, query, carID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []models.CarPhoto
	for rows.Next() {
		var photo models.CarPhoto
		err := rows.Scan(
			&photo.ID, &photo.CarID, &photo.SlotType, &photo.FilePath, &photo.FileURL,
			&photo.FileSize, &photo.MimeType, &photo.CreatedAt, &photo.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}

	return photos, nil
}

// GetByCarIDAndSlot retrieves a specific photo by car ID and slot type
func (r *CarPhotoRepository) GetByCarIDAndSlot(ctx context.Context, carID uuid.UUID, slotType models.PhotoSlotType) (*models.CarPhoto, error) {
	query := `
		SELECT id, car_id, slot_type, file_path, file_url, file_size, mime_type, created_at, updated_at
		FROM car_photos
		WHERE car_id = $1 AND slot_type = $2
	`

	var photo models.CarPhoto
	err := r.db.Pool.QueryRow(ctx, query, carID, slotType).Scan(
		&photo.ID, &photo.CarID, &photo.SlotType, &photo.FilePath, &photo.FileURL,
		&photo.FileSize, &photo.MimeType, &photo.CreatedAt, &photo.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &photo, nil
}

// GetByID retrieves a photo by its ID
func (r *CarPhotoRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.CarPhoto, error) {
	query := `
		SELECT id, car_id, slot_type, file_path, file_url, file_size, mime_type, created_at, updated_at
		FROM car_photos
		WHERE id = $1
	`

	var photo models.CarPhoto
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&photo.ID, &photo.CarID, &photo.SlotType, &photo.FilePath, &photo.FileURL,
		&photo.FileSize, &photo.MimeType, &photo.CreatedAt, &photo.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &photo, nil
}

// Upsert creates or updates a photo for a specific slot
func (r *CarPhotoRepository) Upsert(ctx context.Context, photo *models.CarPhoto) error {
	query := `
		INSERT INTO car_photos (id, car_id, slot_type, file_path, file_url, file_size, mime_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (car_id, slot_type)
		DO UPDATE SET
			file_path = EXCLUDED.file_path,
			file_url = EXCLUDED.file_url,
			file_size = EXCLUDED.file_size,
			mime_type = EXCLUDED.mime_type,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Pool.Exec(ctx, query,
		photo.ID, photo.CarID, photo.SlotType, photo.FilePath, photo.FileURL,
		photo.FileSize, photo.MimeType, photo.CreatedAt, photo.UpdatedAt,
	)

	return err
}

// Delete deletes a photo by ID
func (r *CarPhotoRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM car_photos WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, id)
	return err
}

// DeleteByCarID deletes all photos for a car
func (r *CarPhotoRepository) DeleteByCarID(ctx context.Context, carID uuid.UUID) error {
	query := `DELETE FROM car_photos WHERE car_id = $1`
	_, err := r.db.Pool.Exec(ctx, query, carID)
	return err
}

// CarDocumentRepository handles car document database operations
type CarDocumentRepository struct {
	db *database.DB
}

func NewCarDocumentRepository(db *database.DB) *CarDocumentRepository {
	return &CarDocumentRepository{db: db}
}

// Create creates a new car document record
func (r *CarDocumentRepository) Create(ctx context.Context, doc *models.CarDocument) error {
	query := `
		INSERT INTO car_documents (id, car_id, document_type, file_name, file_path, file_url, file_size, mime_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		doc.ID, doc.CarID, doc.DocumentType, doc.FileName, doc.FilePath, doc.FileURL,
		doc.FileSize, doc.MimeType, doc.CreatedAt, doc.UpdatedAt,
	)

	return err
}

// GetByCarID retrieves all documents for a car
func (r *CarDocumentRepository) GetByCarID(ctx context.Context, carID uuid.UUID) ([]models.CarDocument, error) {
	query := `
		SELECT id, car_id, document_type, file_name, file_path, file_url, file_size, mime_type, created_at, updated_at
		FROM car_documents
		WHERE car_id = $1
		ORDER BY document_type
	`

	rows, err := r.db.Pool.Query(ctx, query, carID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []models.CarDocument
	for rows.Next() {
		var doc models.CarDocument
		err := rows.Scan(
			&doc.ID, &doc.CarID, &doc.DocumentType, &doc.FileName, &doc.FilePath, &doc.FileURL,
			&doc.FileSize, &doc.MimeType, &doc.CreatedAt, &doc.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// GetByID retrieves a document by its ID
func (r *CarDocumentRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.CarDocument, error) {
	query := `
		SELECT id, car_id, document_type, file_name, file_path, file_url, file_size, mime_type, created_at, updated_at
		FROM car_documents
		WHERE id = $1
	`

	var doc models.CarDocument
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&doc.ID, &doc.CarID, &doc.DocumentType, &doc.FileName, &doc.FilePath, &doc.FileURL,
		&doc.FileSize, &doc.MimeType, &doc.CreatedAt, &doc.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &doc, nil
}

// Delete deletes a document by ID
func (r *CarDocumentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM car_documents WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, id)
	return err
}

// DeleteByCarID deletes all documents for a car
func (r *CarDocumentRepository) DeleteByCarID(ctx context.Context, carID uuid.UUID) error {
	query := `DELETE FROM car_documents WHERE car_id = $1`
	_, err := r.db.Pool.Exec(ctx, query, carID)
	return err
}
