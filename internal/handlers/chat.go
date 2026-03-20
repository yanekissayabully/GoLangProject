package handlers

import (
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
	"github.com/gorilla/websocket"

	"github.com/drivebai/backend/internal/auth"
	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
	"github.com/drivebai/backend/internal/ws"
)

type ChatHandler struct {
	chatRepo  *repository.ChatRepository
	uploadDir string
	wsHub     *ws.Hub
	jwtSvc    *auth.JWTService
	logger    *slog.Logger
}

func NewChatHandler(chatRepo *repository.ChatRepository, uploadDir string, wsHub *ws.Hub, jwtSvc *auth.JWTService, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{
		chatRepo:  chatRepo,
		uploadDir: uploadDir,
		wsHub:     wsHub,
		jwtSvc:    jwtSvc,
		logger:    logger,
	}
}

// requireParticipant verifies the caller is a chat participant. Returns false and writes error if not.
func (h *ChatHandler) requireParticipant(w http.ResponseWriter, r *http.Request, chatID, userID uuid.UUID) bool {
	ok, err := h.chatRepo.IsParticipant(r.Context(), chatID, userID)
	if err != nil {
		h.logger.Error("check participant", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return false
	}
	if !ok {
		httputil.WriteError(w, http.StatusForbidden, models.ErrNotParticipant)
		return false
	}
	return true
}

func parseChatID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "chatId"))
}

// --- Chats ---

// FindOrCreateChat creates or fetches a chat for a (car, driver, owner) triple.
func (h *ChatHandler) FindOrCreateChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	var req models.FindOrCreateChatRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}
	if req.CarID == uuid.Nil || req.DriverID == uuid.Nil || req.OwnerID == uuid.Nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("car_id, driver_id, and owner_id are required"))
		return
	}
	// The caller must be one of the participants
	if userID != req.DriverID && userID != req.OwnerID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "You must be the driver or owner"))
		return
	}

	chat, err := h.chatRepo.FindOrCreateChat(r.Context(), req.CarID, req.DriverID, req.OwnerID)
	if err != nil {
		h.logger.Error("find or create chat", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, chat)
}

// ListChats returns all chats for the current user.
func (h *ChatHandler) ListChats(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	archived := r.URL.Query().Get("archived") == "true"

	resp, err := h.chatRepo.ListChatsForUser(r.Context(), userID, archived)
	if err != nil {
		h.logger.Error("list chats", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetChat returns a single chat by ID.
func (h *ChatHandler) GetChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	chat, err := h.chatRepo.GetChatByID(r.Context(), chatID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			httputil.WriteError(w, http.StatusNotFound, apiErr)
		} else {
			h.logger.Error("get chat", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, chat)
}

// --- Messages ---

// ListMessages returns paginated messages for a chat.
func (h *ChatHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 30 // default

	resp, err := h.chatRepo.ListMessages(r.Context(), chatID, cursor, limit)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			httputil.WriteError(w, http.StatusBadRequest, apiErr)
		} else {
			h.logger.Error("list messages", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// SendMessage creates a new message in a chat.
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	var body models.SendMessageRequestBody
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}
	if strings.TrimSpace(body.Body) == "" {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Message body is required"))
		return
	}

	now := time.Now().UTC()
	msg := &models.Message{
		ID:              uuid.New(),
		ChatID:          chatID,
		SenderID:        userID,
		Type:            models.MessageTypeText,
		Body:            body.Body,
		ClientMessageID: &body.ClientMessageID,
		CreatedAt:       now,
	}

	if err := h.chatRepo.CreateMessage(r.Context(), msg); err != nil {
		h.logger.Error("create message", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Look up sender name for the response
	senderName := "Unknown"
	chat, _ := h.chatRepo.GetChatByID(r.Context(), chatID)

	resp := models.MessageResponse{
		ID:              msg.ID,
		ChatID:          msg.ChatID,
		SenderID:        msg.SenderID,
		SenderName:      senderName,
		Type:            msg.Type,
		Body:            msg.Body,
		ClientMessageID: msg.ClientMessageID,
		Attachments:     make([]models.AttachmentResponse, 0),
		CreatedAt:       models.RFC3339Time(msg.CreatedAt),
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)

	// Broadcast to other participants via WebSocket
	if chat != nil {
		targets := []uuid.UUID{}
		if chat.DriverID != userID {
			targets = append(targets, chat.DriverID)
		}
		if chat.OwnerID != userID {
			targets = append(targets, chat.OwnerID)
		}
		h.wsHub.Broadcast(&ws.Event{
			Type:          "new_message",
			Payload:       resp,
			TargetUserIDs: targets,
		})
	}
}

// MarkRead marks messages as read for the current user.
func (h *ChatHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	if err := h.chatRepo.MarkChatRead(r.Context(), chatID, userID); err != nil {
		h.logger.Error("mark read", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// --- Requests ---

// ListRequests returns requests for a chat.
func (h *ChatHandler) ListRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	statusFilter := r.URL.Query().Get("status")
	var statusPtr *string
	if statusFilter != "" {
		statusPtr = &statusFilter
	}

	requests, err := h.chatRepo.ListRequests(r.Context(), chatID, statusPtr)
	if err != nil {
		h.logger.Error("list requests", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, models.RequestsListResponse{Requests: requests})
}

// CreateRequest creates a new request in a chat.
func (h *ChatHandler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	var body models.CreateRequestBody
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if !body.Type.IsValid() {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request type"))
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Title is required"))
		return
	}

	// Determine the target user (the other participant)
	chat, err := h.chatRepo.GetChatByID(r.Context(), chatID)
	if err != nil {
		h.logger.Error("get chat for request", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	targetUserID := chat.DriverID
	if userID == chat.DriverID {
		targetUserID = chat.OwnerID
	}

	// Set default expiry based on request type, or use provided value
	expiresAt := time.Now().UTC().Add(models.DefaultDeadline(body.Type))
	if body.ExpiresAt != nil {
		// Validate bounds: minimum 10 minutes, maximum 7 days
		minExpiry := time.Now().UTC().Add(10 * time.Minute)
		maxExpiry := time.Now().UTC().Add(7 * 24 * time.Hour)
		if body.ExpiresAt.Before(minExpiry) {
			httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Expiry must be at least 10 minutes from now"))
			return
		}
		if body.ExpiresAt.After(maxExpiry) {
			httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Expiry must be within 7 days"))
			return
		}
		expiresAt = *body.ExpiresAt
	}

	currency := "USD"
	if body.Currency != nil {
		currency = *body.Currency
	}

	now := time.Now().UTC()
	req := &models.Request{
		ID:           uuid.New(),
		ChatID:       chatID,
		Type:         body.Type,
		Status:       models.RequestStatusPending,
		CreatedByID:  userID,
		TargetUserID: targetUserID,
		Title:        body.Title,
		Description:  body.Description,
		Amount:       body.Amount,
		Currency:     currency,
		PayloadJSON:  "{}",
		ExpiresAt:    expiresAt,
		CreatedAt:    now,
	}

	created, err := h.chatRepo.CreateRequest(r.Context(), req)
	if err != nil {
		h.logger.Error("create request", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	resp := models.RequestResponse{
		ID:           created.ID,
		ChatID:       created.ChatID,
		Type:         created.Type,
		Status:       created.Status,
		CreatedByID:  created.CreatedByID,
		TargetUserID: created.TargetUserID,
		Title:        created.Title,
		Description:  created.Description,
		Amount:       created.Amount,
		Currency:     created.Currency,
		Attachments:  make([]models.AttachmentResponse, 0),
		ExpiresAt:    models.RFC3339Time(created.ExpiresAt),
		CreatedAt:    models.RFC3339Time(created.CreatedAt),
		UpdatedAt:    models.RFC3339Time(created.UpdatedAt),
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)

	// Broadcast
	h.wsHub.Broadcast(&ws.Event{
		Type:          "request_created",
		Payload:       resp,
		TargetUserIDs: []uuid.UUID{targetUserID},
	})
}

// RespondToRequest handles Accept/Decline/Cancel.
func (h *ChatHandler) RespondToRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	requestID, err := uuid.Parse(chi.URLParam(r, "requestId"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request ID"))
		return
	}

	var body models.RequestActionBody
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if !body.Action.IsValid() {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid action"))
		return
	}

	updated, err := h.chatRepo.RespondToRequest(r.Context(), requestID, body.Action, userID, body.Note)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			statusCode := http.StatusBadRequest
			if apiErr.Code == models.ErrCodeRequestNotFound {
				statusCode = http.StatusNotFound
			}
			httputil.WriteError(w, statusCode, apiErr)
		} else {
			h.logger.Error("respond to request", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	resp := models.RequestResponse{
		ID:           updated.ID,
		ChatID:       updated.ChatID,
		Type:         updated.Type,
		Status:       updated.Status,
		CreatedByID:  updated.CreatedByID,
		TargetUserID: updated.TargetUserID,
		Title:        updated.Title,
		Description:  updated.Description,
		Amount:       updated.Amount,
		Currency:     updated.Currency,
		Attachments:  make([]models.AttachmentResponse, 0),
		ExpiresAt:    models.RFC3339Time(updated.ExpiresAt),
		CreatedAt:    models.RFC3339Time(updated.CreatedAt),
		UpdatedAt:    models.RFC3339Time(updated.UpdatedAt),
	}
	if updated.ResolvedAt != nil {
		t := models.RFC3339Time(*updated.ResolvedAt)
		resp.ResolvedAt = &t
	}

	httputil.WriteJSON(w, http.StatusOK, resp)

	// Broadcast to the other party
	otherUserID := updated.CreatedByID
	if userID == updated.CreatedByID {
		otherUserID = updated.TargetUserID
	}
	h.wsHub.Broadcast(&ws.Event{
		Type:          "request_updated",
		Payload:       resp,
		TargetUserIDs: []uuid.UUID{otherUserID},
	})
}

// --- Actions (Today tab) ---

// GetMyActions returns all pending action items for the authenticated user.
func (h *ChatHandler) GetMyActions(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	actions, err := h.chatRepo.ListPendingActionsForUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("get my actions", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, models.ActionsListResponse{Actions: actions})
}

// --- Chat Details & Settings ---

// GetChatDetails returns detailed chat info.
func (h *ChatHandler) GetChatDetails(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	details, err := h.chatRepo.GetChatDetails(r.Context(), chatID, userID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			httputil.WriteError(w, http.StatusNotFound, apiErr)
		} else {
			h.logger.Error("get chat details", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, details)
}

// UpdateSettings updates per-participant chat settings.
func (h *ChatHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	var body models.UpdateChatSettingsBody
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if err := h.chatRepo.UpdateChatSettings(r.Context(), chatID, userID, &body); err != nil {
		h.logger.Error("update settings", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Return updated details
	details, err := h.chatRepo.GetChatDetails(r.Context(), chatID, userID)
	if err != nil {
		h.logger.Error("get details after update", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, details)
}

// ArchiveChat archives/unarchives a chat for the current user.
func (h *ChatHandler) ArchiveChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	var body models.ArchiveChatBody
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if err := h.chatRepo.ArchiveChat(r.Context(), chatID, userID, body.Archived); err != nil {
		h.logger.Error("archive chat", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// --- Attachments ---

// ListAttachments returns attachments for a chat, optionally filtered by kind.
func (h *ChatHandler) ListAttachments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	kind := r.URL.Query().Get("kind")
	var kindPtr *string
	if kind != "" {
		kindPtr = &kind
	}

	atts, err := h.chatRepo.ListAttachments(r.Context(), chatID, kindPtr)
	if err != nil {
		h.logger.Error("list attachments", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"attachments": atts})
}

// UploadAttachment handles multipart file upload for chat attachments.
func (h *ChatHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := parseChatID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	if !h.requireParticipant(w, r, chatID, userID) {
		return
	}

	// Parse multipart form (max 20MB)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("File too large or invalid form"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("File is required"))
		return
	}
	defer file.Close()

	// Determine attachment kind from mime type
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	var kind models.AttachmentKind
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		kind = models.AttachmentKindImage
	case strings.HasPrefix(mimeType, "video/"):
		kind = models.AttachmentKindVideo
	default:
		kind = models.AttachmentKindDocument
	}

	// Save file to disk
	attID := uuid.New()
	dir := filepath.Join(h.uploadDir, "chats", chatID.String())
	if err := os.MkdirAll(dir, 0755); err != nil {
		h.logger.Error("create attachment dir", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%s%s", attID.String(), ext)
	filePath := filepath.Join(dir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		h.logger.Error("create attachment file", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		h.logger.Error("write attachment file", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	fileURL := fmt.Sprintf("/uploads/chats/%s/%s", chatID.String(), filename)

	att := &models.Attachment{
		ID:         attID,
		ChatID:     chatID,
		UploaderID: userID,
		Kind:       kind,
		Filename:   header.Filename,
		MimeType:   mimeType,
		FileSize:   int(written),
		FilePath:   filePath,
		FileURL:    fileURL,
		CreatedAt:  time.Now().UTC(),
	}

	if err := h.chatRepo.CreateAttachment(r.Context(), att); err != nil {
		h.logger.Error("save attachment record", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	resp := models.AttachmentResponse{
		ID:        att.ID,
		Kind:      att.Kind,
		Filename:  att.Filename,
		MimeType:  att.MimeType,
		FileSize:  att.FileSize,
		FileURL:   att.FileURL,
		CreatedAt: models.RFC3339Time(att.CreatedAt),
	}

	httputil.WriteJSON(w, http.StatusCreated, resp)
}

// --- User Profile ---

// GetUserProfile returns a user's profile detail.
func (h *ChatHandler) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	targetID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid user ID"))
		return
	}

	profile, err := h.chatRepo.GetUserProfileDetail(r.Context(), targetID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			httputil.WriteError(w, http.StatusNotFound, apiErr)
		} else {
			h.logger.Error("get user profile", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, profile)
}

// --- WebSocket ---

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (same as CORS config)
	},
}

// HandleWebSocket upgrades the HTTP connection to WebSocket.
// Auth is done via ?token= query parameter since WS doesn't support Authorization header.
func (h *ChatHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	claims, err := h.jwtSvc.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("ws upgrade failed", "error", err)
		return
	}

	wsConn := ws.NewConn(h.wsHub, conn, claims.UserID, h.logger)
	h.wsHub.Register(wsConn)

	// Start read/write pumps in goroutines
	go wsConn.WritePump()
	go wsConn.ReadPump()
}
