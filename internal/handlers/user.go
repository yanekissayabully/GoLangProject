package handlers

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type UserHandler struct {
	userRepo *repository.UserRepository
	docRepo  *repository.DocumentRepository
	logger   *slog.Logger
	uploadDir string
}

func NewUserHandler(userRepo *repository.UserRepository, docRepo *repository.DocumentRepository, uploadDir string, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		userRepo:  userRepo,
		docRepo:   docRepo,
		logger:    logger,
		uploadDir: uploadDir,
	}
}

func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			WriteError(w, http.StatusNotFound, apiErr)
		} else {
			h.logger.Error("failed to get user", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	profile := UserProfile{
		ID:               user.ID,
		Email:            user.Email,
		Role:             user.Role,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Phone:            user.Phone,
		IsEmailVerified:  user.IsEmailVerified,
		OnboardingStatus: user.OnboardingStatus,
		ProfilePhotoURL:  user.ProfilePhotoURL,
	}

	WriteJSON(w, http.StatusOK, profile)
}

type UpdateProfileRequest struct {
	Role      *models.Role `json:"role,omitempty"`
	FirstName *string      `json:"first_name,omitempty"`
	LastName  *string      `json:"last_name,omitempty"`
	Phone     *string      `json:"phone,omitempty"`
}

func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	var req UpdateProfileRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_REQUEST", "Invalid request body"))
		return
	}

	// Get current user
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			WriteError(w, http.StatusNotFound, apiErr)
		} else {
			h.logger.Error("failed to get user", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	// Update role if provided
	if req.Role != nil {
		// Validate role - only driver and car_owner allowed via API
		if *req.Role != models.RoleDriver && *req.Role != models.RoleCarOwner {
			WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_ROLE", "Role must be 'driver' or 'car_owner'"))
			return
		}
		if err := h.userRepo.UpdateRole(r.Context(), userID, *req.Role); err != nil {
			h.logger.Error("failed to update role", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
			return
		}
		user.Role = *req.Role
	}

	// Update other fields if provided
	updated := false
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
		updated = true
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
		updated = true
	}
	if req.Phone != nil {
		user.Phone = req.Phone
		updated = true
	}

	if updated {
		if err := h.userRepo.Update(r.Context(), user); err != nil {
			h.logger.Error("failed to update user", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
			return
		}
	}

	// Reload user to get updated data
	user, _ = h.userRepo.GetByID(r.Context(), userID)

	profile := UserProfile{
		ID:               user.ID,
		Email:            user.Email,
		Role:             user.Role,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Phone:            user.Phone,
		IsEmailVerified:  user.IsEmailVerified,
		OnboardingStatus: user.OnboardingStatus,
		ProfilePhotoURL:  user.ProfilePhotoURL,
	}

	WriteSuccess(w, http.StatusOK, "Profile updated successfully", profile)
}

// Document upload handlers

type DocumentResponse struct {
	ID        uuid.UUID             `json:"id"`
	Type      models.DocumentType   `json:"type"`
	FileName  string                `json:"file_name"`
	FileSize  int64                 `json:"file_size"`
	Status    models.DocumentStatus `json:"status"`
	CreatedAt string                `json:"created_at"`
}

func (h *UserHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	// Get document type from URL
	docTypeStr := chi.URLParam(r, "type")
	docType := models.DocumentType(docTypeStr)
	if !docType.IsValid() {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_DOCUMENT_TYPE", "Document type must be 'drivers_license' or 'registration'"))
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_REQUEST", "File too large or invalid form data"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_REQUEST", "File is required"))
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if !isValidDocumentMimeType(contentType) {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_FILE_TYPE", "File must be an image (JPEG, PNG) or PDF"))
		return
	}

	// Create user upload directory
	userDir := filepath.Join(h.uploadDir, userID.String())
	if err := os.MkdirAll(userDir, 0755); err != nil {
		h.logger.Error("failed to create upload directory", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	newFileName := fmt.Sprintf("%s_%s%s", docType, uuid.New().String(), ext)
	filePath := filepath.Join(userDir, newFileName)

	// Save file
	dst, err := os.Create(filePath)
	if err != nil {
		h.logger.Error("failed to create file", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		h.logger.Error("failed to save file", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Delete existing document of same type
	h.docRepo.DeleteByUserIDAndType(r.Context(), userID, docType)

	// Save document record
	doc := &models.Document{
		UserID:   userID,
		Type:     docType,
		FileName: header.Filename,
		FilePath: filePath,
		FileSize: written,
		MimeType: contentType,
		Status:   models.DocumentStatusUploaded,
	}

	if err := h.docRepo.Create(r.Context(), doc); err != nil {
		h.logger.Error("failed to save document record", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Check if user has all required documents and update onboarding status
	hasAllDocs, _ := h.docRepo.HasRequiredDocuments(r.Context(), userID)
	if hasAllDocs {
		h.userRepo.UpdateOnboardingStatus(r.Context(), userID, models.OnboardingDocumentsUploaded)
	}

	WriteJSON(w, http.StatusCreated, DocumentResponse{
		ID:        doc.ID,
		Type:      doc.Type,
		FileName:  doc.FileName,
		FileSize:  doc.FileSize,
		Status:    doc.Status,
		CreatedAt: doc.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *UserHandler) GetDocuments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	docs, err := h.docRepo.GetByUserID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get documents", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	response := make([]DocumentResponse, 0, len(docs))
	for _, doc := range docs {
		response = append(response, DocumentResponse{
			ID:        doc.ID,
			Type:      doc.Type,
			FileName:  doc.FileName,
			FileSize:  doc.FileSize,
			Status:    doc.Status,
			CreatedAt: doc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	WriteJSON(w, http.StatusOK, response)
}

func (h *UserHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	docID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_ID", "Invalid document ID"))
		return
	}

	// Get document to verify ownership
	doc, err := h.docRepo.GetByID(r.Context(), docID)
	if err != nil {
		h.logger.Error("failed to get document", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	if doc == nil {
		WriteError(w, http.StatusNotFound, models.NewAPIError("NOT_FOUND", "Document not found"))
		return
	}

	if doc.UserID != userID {
		WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "Not authorized to delete this document"))
		return
	}

	// Delete file from disk
	os.Remove(doc.FilePath)

	// Delete from database
	if err := h.docRepo.Delete(r.Context(), docID); err != nil {
		h.logger.Error("failed to delete document", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Document deleted"})
}

// Profile photo upload

func (h *UserHandler) UploadProfilePhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	// Parse multipart form (max 5MB for photos)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_REQUEST", "File too large or invalid form data"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_REQUEST", "File is required"))
		return
	}
	defer file.Close()

	// Validate file type (images only)
	contentType := header.Header.Get("Content-Type")
	if !isValidImageMimeType(contentType) {
		WriteError(w, http.StatusBadRequest, models.NewAPIError("INVALID_FILE_TYPE", "File must be an image (JPEG, PNG)"))
		return
	}

	// Get current user to check for existing photo
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Create user upload directory
	userDir := filepath.Join(h.uploadDir, userID.String())
	if err := os.MkdirAll(userDir, 0755); err != nil {
		h.logger.Error("failed to create upload directory", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Delete old profile photo if exists
	if user.ProfilePhotoURL != nil && *user.ProfilePhotoURL != "" {
		oldPath := filepath.Join(h.uploadDir, "..", *user.ProfilePhotoURL)
		os.Remove(oldPath) // Ignore error - old file may not exist
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		// Default to .jpg if no extension
		ext = ".jpg"
	}
	newFileName := fmt.Sprintf("profile_%s%s", uuid.New().String(), ext)
	filePath := filepath.Join(userDir, newFileName)

	// Save file
	dst, err := os.Create(filePath)
	if err != nil {
		h.logger.Error("failed to create file", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		h.logger.Error("failed to save file", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Update user profile photo URL (relative path for now)
	photoURL := fmt.Sprintf("/uploads/%s/%s", userID.String(), newFileName)
	if err := h.userRepo.UpdateProfilePhoto(r.Context(), userID, photoURL); err != nil {
		h.logger.Error("failed to update profile photo", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Update onboarding status if needed
	if user.OnboardingStatus == models.OnboardingRoleSelected || user.OnboardingStatus == models.OnboardingCreated {
		h.userRepo.UpdateOnboardingStatus(r.Context(), userID, models.OnboardingPhotoUploaded)
	}

	// Reload user to get updated data
	user, err = h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to reload user", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Return full user profile (same format as GET /me)
	profile := UserProfile{
		ID:               user.ID,
		Email:            user.Email,
		Role:             user.Role,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Phone:            user.Phone,
		IsEmailVerified:  user.IsEmailVerified,
		OnboardingStatus: user.OnboardingStatus,
		ProfilePhotoURL:  user.ProfilePhotoURL,
	}

	WriteJSON(w, http.StatusOK, profile)
}

// Onboarding completion

func (h *UserHandler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// For drivers, require documents
	if user.Role == models.RoleDriver {
		hasAllDocs, _ := h.docRepo.HasRequiredDocuments(r.Context(), userID)
		if !hasAllDocs {
			WriteError(w, http.StatusBadRequest, models.NewAPIError("INCOMPLETE_ONBOARDING", "Please upload required documents"))
			return
		}
	}

	// Update onboarding status to complete
	if err := h.userRepo.UpdateOnboardingStatus(r.Context(), userID, models.OnboardingComplete); err != nil {
		h.logger.Error("failed to update onboarding status", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Onboarding completed"})
}

// Helper functions

func isValidDocumentMimeType(mimeType string) bool {
	validTypes := []string{"image/jpeg", "image/png", "image/jpg", "application/pdf"}
	for _, t := range validTypes {
		if strings.EqualFold(mimeType, t) {
			return true
		}
	}
	return false
}

func isValidImageMimeType(mimeType string) bool {
	validTypes := []string{"image/jpeg", "image/png", "image/jpg"}
	for _, t := range validTypes {
		if strings.EqualFold(mimeType, t) {
			return true
		}
	}
	return false
}
