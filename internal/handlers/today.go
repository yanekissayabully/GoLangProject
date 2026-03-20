package handlers

import (
	"log/slog"
	"net/http"

	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
)

type TodayHandler struct {
	leaseRepo *repository.LeaseRequestRepository
	userRepo  *repository.UserRepository
	logger    *slog.Logger
}

func NewTodayHandler(
	leaseRepo *repository.LeaseRequestRepository,
	userRepo *repository.UserRepository,
	logger *slog.Logger,
) *TodayHandler {
	return &TodayHandler{
		leaseRepo: leaseRepo,
		userRepo:  userRepo,
		logger:    logger,
	}
}

// GetActions returns actionable items for the current user's Today tab.
// MVP: only lease requests where the user is the owner and status = "requested".
func (h *TodayHandler) GetActions(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	actions, err := h.leaseRepo.ListTodayActionsForOwner(r.Context(), userID)
	if err != nil {
		h.logger.Error("get today actions", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Determine unread status
	lastSeen, err := h.userRepo.GetLastSeenActionsAt(r.Context(), userID)
	if err != nil {
		h.logger.Error("get last seen actions at", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	hasUnread, err := h.leaseRepo.HasUnreadActions(r.Context(), userID, lastSeen)
	if err != nil {
		h.logger.Error("has unread actions", "error", err)
		hasUnread = false // non-fatal
	}

	httputil.WriteJSON(w, http.StatusOK, models.TodayActionsResponse{
		Actions:          actions,
		HasUnreadActions: hasUnread,
	})
}

// MarkActionsSeen sets the user's last_seen_actions_at to now.
func (h *TodayHandler) MarkActionsSeen(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	if err := h.userRepo.UpdateLastSeenActionsAt(r.Context(), userID); err != nil {
		h.logger.Error("mark actions seen", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}
