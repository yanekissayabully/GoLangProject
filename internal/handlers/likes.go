package handlers

import (
	"net/http"
	"strings"

	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type LikesHandler struct {
	likesRepo *repository.LikesRepository
	carRepo   *repository.CarRepository
}

func NewLikesHandler(likesRepo *repository.LikesRepository, carRepo *repository.CarRepository) *LikesHandler {
	return &LikesHandler{
		likesRepo: likesRepo,
		carRepo:   carRepo,
	}
}

// LikedListingsResponse is the response for GET /me/likes
type LikedListingsResponse struct {
	LikedListingIDs []uuid.UUID `json:"liked_listing_ids"`
}

// GetLikedListings returns all listing IDs that the current user has liked
func (h *LikesHandler) GetLikedListings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	ids, err := h.likesRepo.GetLikedListingIDs(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Return empty array instead of null if no likes
	if ids == nil {
		ids = []uuid.UUID{}
	}

	WriteJSON(w, http.StatusOK, LikedListingsResponse{LikedListingIDs: ids})
}

// LikeListing adds a like for the specified listing
func (h *LikesHandler) LikeListing(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	// Parse listing ID from URL - handle both uppercase and lowercase UUIDs
	listingIDStr := chi.URLParam(r, "listingId")
	listingID, err := uuid.Parse(strings.ToLower(listingIDStr))
	if err != nil {
		WriteError(w, http.StatusBadRequest, &models.APIError{
			Code:    "INVALID_LISTING_ID",
			Message: "Invalid listing ID format",
		})
		return
	}

	// Verify listing exists
	car, err := h.carRepo.GetByID(r.Context(), listingID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	if car == nil {
		WriteError(w, http.StatusNotFound, &models.APIError{
			Code:    "LISTING_NOT_FOUND",
			Message: "Listing not found",
		})
		return
	}

	// Add the like
	if err := h.likesRepo.AddLike(r.Context(), userID, listingID); err != nil {
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Listing liked successfully"})
}

// UnlikeListing removes a like for the specified listing
func (h *LikesHandler) UnlikeListing(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	// Parse listing ID from URL - handle both uppercase and lowercase UUIDs
	listingIDStr := chi.URLParam(r, "listingId")
	listingID, err := uuid.Parse(strings.ToLower(listingIDStr))
	if err != nil {
		WriteError(w, http.StatusBadRequest, &models.APIError{
			Code:    "INVALID_LISTING_ID",
			Message: "Invalid listing ID format",
		})
		return
	}

	// Remove the like (doesn't matter if it didn't exist)
	if err := h.likesRepo.RemoveLike(r.Context(), userID, listingID); err != nil {
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Listing unliked successfully"})
}
