package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
)

type CarHandler struct {
	carRepo   *repository.CarRepository
	photoRepo *repository.CarPhotoRepository
	docRepo   *repository.CarDocumentRepository
	userRepo  *repository.UserRepository
	uploadDir string
}

func NewCarHandler(
	carRepo *repository.CarRepository,
	photoRepo *repository.CarPhotoRepository,
	docRepo *repository.CarDocumentRepository,
	userRepo *repository.UserRepository,
	uploadDir string,
) *CarHandler {
	return &CarHandler{
		carRepo:   carRepo,
		photoRepo: photoRepo,
		docRepo:   docRepo,
		userRepo:  userRepo,
		uploadDir: uploadDir,
	}
}

// ListCars returns all cars for the authenticated owner
func (h *CarHandler) ListCars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	cars, err := h.carRepo.GetByOwnerID(ctx, userID)
	if err != nil {
		slog.Error("failed to get cars", "error", err, "error_type", fmt.Sprintf("%T", err), "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Get owner info once
	owner, err := h.userRepo.GetByID(ctx, userID)
	if err != nil {
		slog.Error("failed to get owner", "error", err, "user_id", userID)
	}

	// Build response with photos and documents for each car
	var responses []*models.CarResponse
	for _, car := range cars {
		photos, _ := h.photoRepo.GetByCarID(ctx, car.ID)
		documents, _ := h.docRepo.GetByCarID(ctx, car.ID)
		responses = append(responses, car.ToResponse(photos, documents, owner))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"cars": responses,
	})
}

// GetCar returns a specific car by ID
func (h *CarHandler) GetCar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil {
		slog.Error("failed to get car", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	if car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}

	// Verify ownership
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Get photos, documents, and owner info
	photos, _ := h.photoRepo.GetByCarID(ctx, car.ID)
	documents, _ := h.docRepo.GetByCarID(ctx, car.ID)
	owner, _ := h.userRepo.GetByID(ctx, userID)

	httputil.WriteJSON(w, http.StatusOK, car.ToResponse(photos, documents, owner))
}

// CreateCar creates a new car listing
func (h *CarHandler) CreateCar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	var req models.CreateCarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	// Validate required fields
	if req.Make == "" || req.Model == "" || req.Year == 0 {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Make, model, and year are required"))
		return
	}

	// Generate title if not provided
	title := req.Title
	if title == "" {
		title = fmt.Sprintf("%d %s %s", req.Year, req.Make, req.Model)
	}

	// Create car model
	now := time.Now()
	car := &models.Car{
		ID:          uuid.New(),
		OwnerID:     userID,
		Title:       title,
		Make:        req.Make,
		Model:       req.Model,
		Year:        req.Year,
		BodyType:    req.BodyType,
		FuelType:    req.FuelType,
		Mileage:     req.Mileage,
		IsForRent:   req.IsForRent,
		IsForSale:   req.IsForSale,
		Currency:    "USD",
		Status:      models.CarStatusPending,
		IsPaused:    false,
		RentedWeeks: 0,
		TotalEarned: 0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Set defaults
	if car.BodyType == "" {
		car.BodyType = models.BodyTypeSedan
	}
	if car.FuelType == "" {
		car.FuelType = models.FuelTypeGas
	}

	// Handle optional fields
	if req.Description != nil {
		car.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.Address != nil {
		car.Address = sql.NullString{String: *req.Address, Valid: true}
	}
	if req.Neighborhood != nil {
		car.Neighborhood = sql.NullString{String: *req.Neighborhood, Valid: true}
	}
	if req.Latitude != nil {
		car.Latitude = sql.NullFloat64{Float64: *req.Latitude, Valid: true}
	}
	if req.Longitude != nil {
		car.Longitude = sql.NullFloat64{Float64: *req.Longitude, Valid: true}
	}
	if req.Area != nil {
		car.Area = sql.NullString{String: *req.Area, Valid: true}
	}
	if req.Street != nil {
		car.Street = sql.NullString{String: *req.Street, Valid: true}
	}
	if req.Block != nil {
		car.Block = sql.NullString{String: *req.Block, Valid: true}
	}
	if req.Zip != nil {
		car.Zip = sql.NullString{String: *req.Zip, Valid: true}
	}
	if req.WeeklyRentPrice != nil {
		car.WeeklyRentPrice = sql.NullFloat64{Float64: *req.WeeklyRentPrice, Valid: true}
	}
	if req.SalePrice != nil {
		car.SalePrice = sql.NullFloat64{Float64: *req.SalePrice, Valid: true}
	}
	if req.MinYearsLicensed != nil {
		car.MinYearsLicensed = *req.MinYearsLicensed
	} else {
		car.MinYearsLicensed = 2
	}
	if req.DepositAmount != nil {
		car.DepositAmount = *req.DepositAmount
	} else {
		car.DepositAmount = 500
	}
	if req.InsuranceCoverage != nil {
		car.InsuranceCoverage = *req.InsuranceCoverage
	} else {
		car.InsuranceCoverage = models.InsuranceFullCoverage
	}

	// Save to database
	if err := h.carRepo.Create(ctx, car); err != nil {
		slog.Error("failed to create car", "error", err, "error_type", fmt.Sprintf("%T", err), "user_id", userID, "car_id", car.ID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Get owner info for response
	owner, _ := h.userRepo.GetByID(ctx, userID)

	slog.Info("car created", "car_id", car.ID, "user_id", userID)
	httputil.WriteJSON(w, http.StatusCreated, car.ToResponse(nil, nil, owner))
}

// UpdateCar updates an existing car listing
func (h *CarHandler) UpdateCar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Get existing car
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}

	// Verify ownership
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	var req models.UpdateCarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	// Apply updates
	if req.Title != nil {
		car.Title = *req.Title
	}
	if req.Description != nil {
		car.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.Make != nil {
		car.Make = *req.Make
	}
	if req.Model != nil {
		car.Model = *req.Model
	}
	if req.Year != nil {
		car.Year = *req.Year
	}
	if req.BodyType != nil {
		car.BodyType = *req.BodyType
	}
	if req.FuelType != nil {
		car.FuelType = *req.FuelType
	}
	if req.Mileage != nil {
		car.Mileage = *req.Mileage
	}
	if req.Address != nil {
		car.Address = sql.NullString{String: *req.Address, Valid: true}
	}
	if req.Neighborhood != nil {
		car.Neighborhood = sql.NullString{String: *req.Neighborhood, Valid: true}
	}
	if req.Latitude != nil {
		car.Latitude = sql.NullFloat64{Float64: *req.Latitude, Valid: true}
	}
	if req.Longitude != nil {
		car.Longitude = sql.NullFloat64{Float64: *req.Longitude, Valid: true}
	}
	if req.Area != nil {
		car.Area = sql.NullString{String: *req.Area, Valid: true}
	}
	if req.Street != nil {
		car.Street = sql.NullString{String: *req.Street, Valid: true}
	}
	if req.Block != nil {
		car.Block = sql.NullString{String: *req.Block, Valid: true}
	}
	if req.Zip != nil {
		car.Zip = sql.NullString{String: *req.Zip, Valid: true}
	}
	if req.IsForRent != nil {
		car.IsForRent = *req.IsForRent
	}
	if req.WeeklyRentPrice != nil {
		car.WeeklyRentPrice = sql.NullFloat64{Float64: *req.WeeklyRentPrice, Valid: true}
	}
	if req.IsForSale != nil {
		car.IsForSale = *req.IsForSale
	}
	if req.SalePrice != nil {
		car.SalePrice = sql.NullFloat64{Float64: *req.SalePrice, Valid: true}
	}
	if req.MinYearsLicensed != nil {
		car.MinYearsLicensed = *req.MinYearsLicensed
	}
	if req.DepositAmount != nil {
		car.DepositAmount = *req.DepositAmount
	}
	if req.InsuranceCoverage != nil {
		car.InsuranceCoverage = *req.InsuranceCoverage
	}
	if req.Status != nil {
		car.Status = *req.Status
	}
	if req.IsPaused != nil {
		car.IsPaused = *req.IsPaused
		if *req.IsPaused {
			car.Status = models.CarStatusPaused
		} else if car.Status == models.CarStatusPaused {
			car.Status = models.CarStatusAvailable
		}
	}

	// Save to database
	if err := h.carRepo.Update(ctx, car); err != nil {
		slog.Error("failed to update car", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Get photos, documents, and owner info for response
	photos, _ := h.photoRepo.GetByCarID(ctx, car.ID)
	documents, _ := h.docRepo.GetByCarID(ctx, car.ID)
	owner, _ := h.userRepo.GetByID(ctx, userID)

	slog.Info("car updated", "car_id", car.ID, "user_id", userID)
	httputil.WriteJSON(w, http.StatusOK, car.ToResponse(photos, documents, owner))
}

// DeleteCar deletes a car listing
func (h *CarHandler) DeleteCar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Get existing car
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}

	// Verify ownership
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Delete photos from disk
	photos, _ := h.photoRepo.GetByCarID(ctx, carID)
	for _, photo := range photos {
		os.Remove(photo.FilePath)
	}

	// Delete documents from disk
	documents, _ := h.docRepo.GetByCarID(ctx, carID)
	for _, doc := range documents {
		os.Remove(doc.FilePath)
	}

	// Delete car (cascades to photos and documents)
	if err := h.carRepo.Delete(ctx, carID); err != nil {
		slog.Error("failed to delete car", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	slog.Info("car deleted", "car_id", carID, "user_id", userID)
	httputil.WriteSuccess(w, http.StatusOK, "Car deleted successfully", nil)
}

// PauseCar toggles the paused state of a car
func (h *CarHandler) PauseCar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Get existing car
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}

	// Verify ownership
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Toggle pause state
	newIsPaused := !car.IsPaused
	newStatus := models.CarStatusAvailable
	if newIsPaused {
		newStatus = models.CarStatusPaused
	}

	if err := h.carRepo.UpdateStatus(ctx, carID, newStatus, newIsPaused); err != nil {
		slog.Error("failed to pause car", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Get updated car
	car, _ = h.carRepo.GetByID(ctx, carID)
	photos, _ := h.photoRepo.GetByCarID(ctx, car.ID)
	documents, _ := h.docRepo.GetByCarID(ctx, car.ID)
	owner, _ := h.userRepo.GetByID(ctx, userID)

	slog.Info("car paused toggled", "car_id", carID, "is_paused", newIsPaused)
	httputil.WriteJSON(w, http.StatusOK, car.ToResponse(photos, documents, owner))
}

// ListCarPhotos returns all photos for a car
func (h *CarHandler) ListCarPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Verify car exists and ownership
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	photos, err := h.photoRepo.GetByCarID(ctx, carID)
	if err != nil {
		slog.Error("failed to get car photos", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	var photoResponses []models.CarPhotoResponse
	for _, p := range photos {
		photoResponses = append(photoResponses, models.CarPhotoResponse{
			ID:        p.ID,
			SlotType:  p.SlotType,
			FileURL:   p.FileURL,
			FileSize:  p.FileSize,
			CreatedAt: models.RFC3339Time(p.CreatedAt),
			UpdatedAt: models.RFC3339Time(p.UpdatedAt),
		})
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"photos": photoResponses,
	})
}

// UploadCarPhoto uploads a photo for a specific slot
func (h *CarHandler) UploadCarPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Verify car exists and ownership
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Failed to parse form data"))
		return
	}

	// Get slot type
	slotTypeStr := r.FormValue("slot_type")
	if slotTypeStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("slot_type is required"))
		return
	}

	slotType := models.PhotoSlotType(slotTypeStr)
	validSlots := map[models.PhotoSlotType]bool{
		models.PhotoSlotCoverFront: true,
		models.PhotoSlotRight:      true,
		models.PhotoSlotLeft:       true,
		models.PhotoSlotBack:       true,
		models.PhotoSlotDashboard:  true,
	}
	if !validSlots[slotType] {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid slot_type"))
		return
	}

	// Get file
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("file is required"))
		return
	}
	defer file.Close()

	// Validate mime type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		// Try to detect from file
		buffer := make([]byte, 512)
		file.Read(buffer)
		contentType = http.DetectContentType(buffer)
		file.Seek(0, 0)
	}

	validTypes := map[string]string{
		"image/jpeg": ".jpg",
		"image/jpg":  ".jpg",
		"image/png":  ".png",
	}
	ext, valid := validTypes[contentType]
	if !valid {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Only JPEG and PNG images are allowed"))
		return
	}

	// Delete existing photo for this slot if exists
	existingPhoto, _ := h.photoRepo.GetByCarIDAndSlot(ctx, carID, slotType)
	if existingPhoto != nil {
		os.Remove(existingPhoto.FilePath)
	}

	// Create directory for car photos
	carDir := filepath.Join(h.uploadDir, "cars", carID.String())
	if err := os.MkdirAll(carDir, 0755); err != nil {
		slog.Error("failed to create car directory", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Generate unique filename
	photoID := uuid.New()
	filename := fmt.Sprintf("%s_%s%s", slotTypeStr, photoID.String(), ext)
	filePath := filepath.Join(carDir, filename)

	// Save file to disk
	dst, err := os.Create(filePath)
	if err != nil {
		slog.Error("failed to create file", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	defer dst.Close()

	fileSize, err := io.Copy(dst, file)
	if err != nil {
		slog.Error("failed to write file", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Create photo URL
	fileURL := fmt.Sprintf("/uploads/cars/%s/%s", carID.String(), filename)

	// Create or update photo record
	now := time.Now()
	photo := &models.CarPhoto{
		ID:        photoID,
		CarID:     carID,
		SlotType:  slotType,
		FilePath:  filePath,
		FileURL:   fileURL,
		FileSize:  int(fileSize),
		MimeType:  contentType,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.photoRepo.Upsert(ctx, photo); err != nil {
		slog.Error("failed to save photo record", "error", err)
		os.Remove(filePath) // Clean up file
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Auto-publish listing when first photo (cover_front) is uploaded
	// This makes the listing visible in Discover for drivers
	slog.Info("checking auto-publish conditions",
		"slot_type", slotType,
		"expected_slot", models.PhotoSlotCoverFront,
		"car_status", car.Status,
		"expected_status", models.CarStatusPending,
		"will_auto_publish", slotType == models.PhotoSlotCoverFront && car.Status == models.CarStatusPending)

	if slotType == models.PhotoSlotCoverFront && car.Status == models.CarStatusPending {
		slog.Info("auto-publishing car", "car_id", carID, "old_status", car.Status)
		if err := h.carRepo.UpdateStatus(ctx, carID, models.CarStatusAvailable, false); err != nil {
			slog.Warn("failed to auto-publish car after photo upload", "error", err, "car_id", carID)
			// Don't fail the request - photo was uploaded successfully
		} else {
			slog.Info("car auto-published after cover photo upload", "car_id", carID, "new_status", models.CarStatusAvailable)
		}
	}

	slog.Info("car photo uploaded", "car_id", carID, "slot_type", slotType, "photo_id", photoID)
	httputil.WriteJSON(w, http.StatusOK, models.CarPhotoResponse{
		ID:        photo.ID,
		SlotType:  photo.SlotType,
		FileURL:   photo.FileURL,
		FileSize:  photo.FileSize,
		CreatedAt: models.RFC3339Time(photo.CreatedAt),
		UpdatedAt: models.RFC3339Time(photo.UpdatedAt),
	})
}

// DeleteCarPhoto deletes a car photo
func (h *CarHandler) DeleteCarPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	photoIDStr := chi.URLParam(r, "photoId")
	photoID, err := uuid.Parse(photoIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid photo ID"))
		return
	}

	// Verify car exists and ownership
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Get photo
	photo, err := h.photoRepo.GetByID(ctx, photoID)
	if err != nil || photo == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Photo not found"))
		return
	}

	// Verify photo belongs to this car
	if photo.CarID != carID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "Photo does not belong to this car"))
		return
	}

	// Delete file from disk
	os.Remove(photo.FilePath)

	// Delete from database
	if err := h.photoRepo.Delete(ctx, photoID); err != nil {
		slog.Error("failed to delete photo", "error", err, "photo_id", photoID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	slog.Info("car photo deleted", "car_id", carID, "photo_id", photoID)
	httputil.WriteSuccess(w, http.StatusOK, "Photo deleted successfully", nil)
}

// ListCarDocuments returns all documents for a car
func (h *CarHandler) ListCarDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Verify car exists and ownership
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	documents, err := h.docRepo.GetByCarID(ctx, carID)
	if err != nil {
		slog.Error("failed to get car documents", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	var docResponses []models.CarDocumentResponse
	for _, d := range documents {
		docResponses = append(docResponses, models.CarDocumentResponse{
			ID:           d.ID,
			DocumentType: d.DocumentType,
			FileName:     d.FileName,
			FileURL:      d.FileURL,
			FileSize:     d.FileSize,
			CreatedAt:    models.RFC3339Time(d.CreatedAt),
			UpdatedAt:    models.RFC3339Time(d.UpdatedAt),
		})
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"documents": docResponses,
	})
}

// UploadCarDocument uploads a document for a car
func (h *CarHandler) UploadCarDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Verify car exists and ownership
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Failed to parse form data"))
		return
	}

	// Get document type
	docTypeStr := r.FormValue("document_type")
	if docTypeStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("document_type is required"))
		return
	}

	docType := models.CarDocumentType(docTypeStr)
	validTypes := map[models.CarDocumentType]bool{
		models.CarDocInspection:   true,
		models.CarDocRegistration: true,
		models.CarDocPermit:       true,
		models.CarDocInsurance:    true,
	}
	if !validTypes[docType] {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid document_type"))
		return
	}

	// Get file
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("file is required"))
		return
	}
	defer file.Close()

	// Validate mime type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		buffer := make([]byte, 512)
		file.Read(buffer)
		contentType = http.DetectContentType(buffer)
		file.Seek(0, 0)
	}

	validMimeTypes := map[string]string{
		"image/jpeg":      ".jpg",
		"image/jpg":       ".jpg",
		"image/png":       ".png",
		"application/pdf": ".pdf",
	}
	ext, valid := validMimeTypes[contentType]
	if !valid {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Only JPEG, PNG, and PDF files are allowed"))
		return
	}

	// Create directory for car documents
	carDir := filepath.Join(h.uploadDir, "cars", carID.String(), "documents")
	if err := os.MkdirAll(carDir, 0755); err != nil {
		slog.Error("failed to create car directory", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Generate unique filename
	docID := uuid.New()
	originalName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	filename := fmt.Sprintf("%s_%s_%s%s", docTypeStr, originalName, docID.String()[:8], ext)
	filePath := filepath.Join(carDir, filename)

	// Save file to disk
	dst, err := os.Create(filePath)
	if err != nil {
		slog.Error("failed to create file", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	defer dst.Close()

	fileSize, err := io.Copy(dst, file)
	if err != nil {
		slog.Error("failed to write file", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Create document URL
	fileURL := fmt.Sprintf("/uploads/cars/%s/documents/%s", carID.String(), filename)

	// Create document record
	now := time.Now()
	doc := &models.CarDocument{
		ID:           docID,
		CarID:        carID,
		DocumentType: docType,
		FileName:     header.Filename,
		FilePath:     filePath,
		FileURL:      fileURL,
		FileSize:     int(fileSize),
		MimeType:     contentType,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.docRepo.Create(ctx, doc); err != nil {
		slog.Error("failed to save document record", "error", err)
		os.Remove(filePath)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	slog.Info("car document uploaded", "car_id", carID, "doc_type", docType, "doc_id", docID)
	httputil.WriteJSON(w, http.StatusOK, models.CarDocumentResponse{
		ID:           doc.ID,
		DocumentType: doc.DocumentType,
		FileName:     doc.FileName,
		FileURL:      doc.FileURL,
		FileSize:     doc.FileSize,
		CreatedAt:    models.RFC3339Time(doc.CreatedAt),
		UpdatedAt:    models.RFC3339Time(doc.UpdatedAt),
	})
}

// ListAvailableListings returns all available cars for drivers to browse (public endpoint)
func (h *CarHandler) ListAvailableListings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get query parameters for filtering
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "available"
	}

	search := r.URL.Query().Get("search")

	cars, err := h.carRepo.GetAvailableListings(ctx, status, search)
	if err != nil {
		slog.Error("failed to get available listings", "error", err, "error_type", fmt.Sprintf("%T", err), "status", status, "search", search)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Build response with photos and owner info for each car
	var responses []*models.CarResponse
	for _, car := range cars {
		photos, _ := h.photoRepo.GetByCarID(ctx, car.ID)
		owner, _ := h.userRepo.GetByID(ctx, car.OwnerID)
		responses = append(responses, car.ToResponse(photos, nil, owner))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"listings": responses,
		"count":    len(responses),
	})
}

// UpdateCarLocation updates only the location of a car (owner-only)
func (h *CarHandler) UpdateCarLocation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	// Get existing car
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}

	// Verify ownership
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	var req models.UpdateCarLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	// Validate lat/lng
	if req.Latitude == nil || req.Longitude == nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("latitude and longitude are required"))
		return
	}
	if *req.Latitude < -90 || *req.Latitude > 90 {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("latitude must be between -90 and 90"))
		return
	}
	if *req.Longitude < -180 || *req.Longitude > 180 {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("longitude must be between -180 and 180"))
		return
	}

	area := ""
	if req.Area != nil {
		area = *req.Area
	}
	street := ""
	if req.Street != nil {
		street = *req.Street
	}
	block := ""
	if req.Block != nil {
		block = *req.Block
	}
	zip := ""
	if req.Zip != nil {
		zip = *req.Zip
	}

	slog.Info("updating car location", "car_id", carID, "lat", *req.Latitude, "lng", *req.Longitude, "area", area, "street", street)

	if err := h.carRepo.UpdateLocation(ctx, carID, *req.Latitude, *req.Longitude, area, street, block, zip); err != nil {
		slog.Error("failed to update car location", "error", err, "car_id", carID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Fetch updated car for response
	car, _ = h.carRepo.GetByID(ctx, carID)
	photos, _ := h.photoRepo.GetByCarID(ctx, car.ID)
	documents, _ := h.docRepo.GetByCarID(ctx, car.ID)
	owner, _ := h.userRepo.GetByID(ctx, userID)

	slog.Info("car location updated", "car_id", carID, "user_id", userID)
	httputil.WriteJSON(w, http.StatusOK, car.ToResponse(photos, documents, owner))
}

// DeleteCarDocument deletes a car document
func (h *CarHandler) DeleteCarDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.GetUserID(ctx)
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	carIDStr := chi.URLParam(r, "carId")
	carID, err := uuid.Parse(carIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid car ID"))
		return
	}

	docIDStr := chi.URLParam(r, "docId")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid document ID"))
		return
	}

	// Verify car exists and ownership
	car, err := h.carRepo.GetByID(ctx, carID)
	if err != nil || car == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Car not found"))
		return
	}
	if car.OwnerID != userID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You do not own this car"))
		return
	}

	// Get document
	doc, err := h.docRepo.GetByID(ctx, docID)
	if err != nil || doc == nil {
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Document not found"))
		return
	}

	// Verify document belongs to this car
	if doc.CarID != carID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "Document does not belong to this car"))
		return
	}

	// Delete file from disk
	os.Remove(doc.FilePath)

	// Delete from database
	if err := h.docRepo.Delete(ctx, docID); err != nil {
		slog.Error("failed to delete document", "error", err, "doc_id", docID)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	slog.Info("car document deleted", "car_id", carID, "doc_id", docID)
	httputil.WriteSuccess(w, http.StatusOK, "Document deleted successfully", nil)
}
