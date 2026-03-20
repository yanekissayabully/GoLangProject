package repository

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
)

type ChatRepository struct {
	db *database.DB
}

func NewChatRepository(db *database.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

// FindOrCreateChat returns an existing chat or creates a new one for the (car, driver, owner) triple.
func (r *ChatRepository) FindOrCreateChat(ctx context.Context, carID, driverID, ownerID uuid.UUID) (*models.Chat, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	chatID := uuid.New()
	now := time.Now().UTC()

	// INSERT ON CONFLICT DO NOTHING
	_, err = tx.Exec(ctx, `
		INSERT INTO chats (id, car_id, driver_id, owner_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (car_id, driver_id, owner_id) DO NOTHING
	`, chatID, carID, driverID, ownerID, now)
	if err != nil {
		return nil, fmt.Errorf("insert chat: %w", err)
	}

	// SELECT the chat (whether it was just created or already existed)
	var chat models.Chat
	err = tx.QueryRow(ctx, `
		SELECT id, car_id, driver_id, owner_id, last_message_at, last_request_at, created_at, updated_at
		FROM chats WHERE car_id = $1 AND driver_id = $2 AND owner_id = $3
	`, carID, driverID, ownerID).Scan(
		&chat.ID, &chat.CarID, &chat.DriverID, &chat.OwnerID,
		&chat.LastMessageAt, &chat.LastRequestAt, &chat.CreatedAt, &chat.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("select chat: %w", err)
	}

	// Ensure participants exist for both users
	for _, userID := range []uuid.UUID{driverID, ownerID} {
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_participants (id, chat_id, user_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4)
			ON CONFLICT (chat_id, user_id) DO NOTHING
		`, uuid.New(), chat.ID, userID, now)
		if err != nil {
			return nil, fmt.Errorf("insert participant %s: %w", userID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &chat, nil
}

// IsParticipant checks if userID is a participant in chatID.
func (r *ChatRepository) IsParticipant(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM chat_participants WHERE chat_id = $1 AND user_id = $2)
	`, chatID, userID).Scan(&exists)
	return exists, err
}

// GetChatByID returns a chat by its ID.
func (r *ChatRepository) GetChatByID(ctx context.Context, chatID uuid.UUID) (*models.Chat, error) {
	var chat models.Chat
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, car_id, driver_id, owner_id, last_message_at, last_request_at, created_at, updated_at
		FROM chats WHERE id = $1
	`, chatID).Scan(
		&chat.ID, &chat.CarID, &chat.DriverID, &chat.OwnerID,
		&chat.LastMessageAt, &chat.LastRequestAt, &chat.CreatedAt, &chat.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrChatNotFound
	}
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// ListChatsForUser returns all chats for a user with computed counts.
func (r *ChatRepository) ListChatsForUser(ctx context.Context, userID uuid.UUID, archived bool) (*models.ChatsListResponse, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			c.id,
			c.car_id,
			car.title AS car_title,
			(SELECT cp2.file_url FROM car_photos cp2 WHERE cp2.car_id = c.car_id AND cp2.slot_type = 'cover_front' LIMIT 1) AS car_cover_photo_url,
			CASE WHEN c.driver_id = $1 THEN c.owner_id ELSE c.driver_id END AS counterparty_id,
			CASE WHEN c.driver_id = $1
				THEN (SELECT first_name || ' ' || last_name FROM users WHERE id = c.owner_id)
				ELSE (SELECT first_name || ' ' || last_name FROM users WHERE id = c.driver_id)
			END AS counterparty_name,
			CASE WHEN c.driver_id = $1
				THEN (SELECT profile_photo_url FROM users WHERE id = c.owner_id)
				ELSE (SELECT profile_photo_url FROM users WHERE id = c.driver_id)
			END AS counterparty_avatar_url,
			(SELECT body FROM messages WHERE chat_id = c.id ORDER BY created_at DESC LIMIT 1) AS last_message,
			c.last_message_at,
			COALESCE((
				SELECT COUNT(*) FROM messages m
				WHERE m.chat_id = c.id AND m.created_at > cp.last_read_at AND m.sender_id != $1
			), 0) AS unread_count,
			COALESCE((
				SELECT COUNT(*) FROM requests req
				WHERE req.chat_id = c.id AND req.status = 'pending' AND req.expires_at > NOW()
			), 0) AS open_requests_count,
			cp.is_archived
		FROM chats c
		JOIN chat_participants cp ON cp.chat_id = c.id AND cp.user_id = $1
		JOIN cars car ON car.id = c.car_id
		WHERE cp.is_archived = $2
		ORDER BY COALESCE(c.last_message_at, c.created_at) DESC
	`, userID, archived)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	var totalUnread int
	items := make([]models.ChatListItemResponse, 0)
	for rows.Next() {
		var item models.ChatListItemResponse
		var lastMsgAt *time.Time
		err := rows.Scan(
			&item.ID, &item.CarID, &item.CarTitle, &item.CarCoverPhotoURL,
			&item.CounterpartyID, &item.CounterpartyName, &item.CounterpartyAvatarURL,
			&item.LastMessage, &lastMsgAt,
			&item.UnreadCount, &item.OpenRequestsCount, &item.IsArchived,
		)
		if err != nil {
			return nil, fmt.Errorf("scan chat row: %w", err)
		}
		if lastMsgAt != nil {
			t := models.RFC3339Time(*lastMsgAt)
			item.LastMessageAt = &t
		}
		totalUnread += item.UnreadCount
		items = append(items, item)
	}

	return &models.ChatsListResponse{
		Chats:       items,
		TotalUnread: totalUnread,
	}, nil
}

// ListMessages returns messages for a chat with cursor-based pagination.
// Cursor is base64-encoded created_at timestamp. Messages returned newest-first.
func (r *ChatRepository) ListMessages(ctx context.Context, chatID uuid.UUID, cursor string, limit int) (*models.MessagesPageResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	var rows pgx.Rows
	var err error

	if cursor != "" {
		cursorBytes, decErr := base64.StdEncoding.DecodeString(cursor)
		if decErr != nil {
			return nil, models.NewValidationError("Invalid cursor")
		}
		cursorTime, parseErr := time.Parse(time.RFC3339Nano, string(cursorBytes))
		if parseErr != nil {
			return nil, models.NewValidationError("Invalid cursor")
		}
		rows, err = r.db.Pool.Query(ctx, `
			SELECT m.id, m.chat_id, m.sender_id,
				(SELECT first_name || ' ' || last_name FROM users WHERE id = m.sender_id) AS sender_name,
				m.type, m.body, m.client_message_id, m.request_id, m.created_at
			FROM messages m
			WHERE m.chat_id = $1 AND m.created_at < $2
			ORDER BY m.created_at DESC
			LIMIT $3
		`, chatID, cursorTime, limit+1)
	} else {
		rows, err = r.db.Pool.Query(ctx, `
			SELECT m.id, m.chat_id, m.sender_id,
				(SELECT first_name || ' ' || last_name FROM users WHERE id = m.sender_id) AS sender_name,
				m.type, m.body, m.client_message_id, m.request_id, m.created_at
			FROM messages m
			WHERE m.chat_id = $1
			ORDER BY m.created_at DESC
			LIMIT $2
		`, chatID, limit+1)
	}
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	msgs := make([]models.MessageResponse, 0)
	for rows.Next() {
		var msg models.MessageResponse
		var createdAt time.Time
		err := rows.Scan(
			&msg.ID, &msg.ChatID, &msg.SenderID, &msg.SenderName,
			&msg.Type, &msg.Body, &msg.ClientMessageID, &msg.RequestID, &createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msg.CreatedAt = models.RFC3339Time(createdAt)
		msg.Attachments = make([]models.AttachmentResponse, 0)
		msgs = append(msgs, msg)
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	var nextCursor *string
	if hasMore && len(msgs) > 0 {
		lastTime := time.Time(msgs[len(msgs)-1].CreatedAt)
		encoded := base64.StdEncoding.EncodeToString([]byte(lastTime.Format(time.RFC3339Nano)))
		nextCursor = &encoded
	}

	// Load attachments for these messages
	if len(msgs) > 0 {
		msgIDs := make([]uuid.UUID, len(msgs))
		for i, m := range msgs {
			msgIDs[i] = m.ID
		}
		attachmentMap, err := r.getAttachmentsForMessages(ctx, msgIDs)
		if err != nil {
			return nil, err
		}
		for i := range msgs {
			if atts, ok := attachmentMap[msgs[i].ID]; ok {
				msgs[i].Attachments = atts
			}
		}
	}

	return &models.MessagesPageResponse{
		Messages:   msgs,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r *ChatRepository) getAttachmentsForMessages(ctx context.Context, msgIDs []uuid.UUID) (map[uuid.UUID][]models.AttachmentResponse, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, message_id, kind, filename, mime_type, file_size, file_url, created_at
		FROM attachments
		WHERE message_id = ANY($1)
		ORDER BY created_at ASC
	`, msgIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]models.AttachmentResponse)
	for rows.Next() {
		var a models.AttachmentResponse
		var msgID uuid.UUID
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &msgID, &a.Kind, &a.Filename, &a.MimeType, &a.FileSize, &a.FileURL, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = models.RFC3339Time(createdAt)
		result[msgID] = append(result[msgID], a)
	}
	return result, nil
}

// CreateMessage creates a new message and updates the chat's last_message_at.
func (r *ChatRepository) CreateMessage(ctx context.Context, msg *models.Message) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, type, body, client_message_id, request_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, msg.ID, msg.ChatID, msg.SenderID, msg.Type, msg.Body, msg.ClientMessageID, msg.RequestID, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE chats SET last_message_at = $2 WHERE id = $1
	`, msg.ChatID, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("update chat last_message_at: %w", err)
	}

	return tx.Commit(ctx)
}

// MarkChatRead updates the participant's last_read_at to now.
func (r *ChatRepository) MarkChatRead(ctx context.Context, chatID, userID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE chat_participants SET last_read_at = NOW() WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID)
	return err
}

// CreateRequest creates a new request, updates chat, and creates a system message.
func (r *ChatRepository) CreateRequest(ctx context.Context, req *models.Request) (*models.Request, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO requests (id, chat_id, type, status, created_by_id, target_user_id,
			title, description, amount, currency, payload_json, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
		RETURNING id, chat_id, type, status, created_by_id, target_user_id,
			title, description, amount, currency, payload_json, expires_at, resolved_at, created_at, updated_at
	`, req.ID, req.ChatID, req.Type, models.RequestStatusPending, req.CreatedByID, req.TargetUserID,
		req.Title, req.Description, req.Amount, req.Currency, req.PayloadJSON, req.ExpiresAt, req.CreatedAt,
	).Scan(
		&req.ID, &req.ChatID, &req.Type, &req.Status, &req.CreatedByID, &req.TargetUserID,
		&req.Title, &req.Description, &req.Amount, &req.Currency, &req.PayloadJSON,
		&req.ExpiresAt, &req.ResolvedAt, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert request: %w", err)
	}

	// Update chat last_request_at
	_, err = tx.Exec(ctx, `UPDATE chats SET last_request_at = $2 WHERE id = $1`, req.ChatID, req.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Create system message about the request
	sysMsg := fmt.Sprintf("New request: %s", req.Title)
	_, err = tx.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, type, body, request_id, created_at)
		VALUES ($1, $2, $3, 'system', $4, $5, $6)
	`, uuid.New(), req.ChatID, req.CreatedByID, sysMsg, req.ID, req.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert system message: %w", err)
	}

	// Also update last_message_at
	_, err = tx.Exec(ctx, `UPDATE chats SET last_message_at = $2 WHERE id = $1`, req.ChatID, req.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return req, nil
}

// RespondToRequest handles Accept/Decline/Cancel with state machine validation.
func (r *ChatRepository) RespondToRequest(ctx context.Context, requestID uuid.UUID, action models.RequestAction, responderID uuid.UUID, note *string) (*models.Request, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock and fetch the request
	var req models.Request
	err = tx.QueryRow(ctx, `
		SELECT id, chat_id, type, status, created_by_id, target_user_id,
			title, description, amount, currency, payload_json, expires_at, resolved_at, created_at, updated_at
		FROM requests
		WHERE id = $1
		FOR UPDATE
	`, requestID).Scan(
		&req.ID, &req.ChatID, &req.Type, &req.Status, &req.CreatedByID, &req.TargetUserID,
		&req.Title, &req.Description, &req.Amount, &req.Currency, &req.PayloadJSON,
		&req.ExpiresAt, &req.ResolvedAt, &req.CreatedAt, &req.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}

	// Must be pending
	if req.Status != models.RequestStatusPending {
		return nil, models.ErrInvalidAction
	}

	// Check expiry
	if req.IsExpired() {
		// Auto-expire it
		_, _ = tx.Exec(ctx, `UPDATE requests SET status = 'expired', updated_at = NOW() WHERE id = $1`, requestID)
		_ = tx.Commit(ctx)
		return nil, models.ErrRequestExpired
	}

	// Validate who can perform which action
	var newStatus models.RequestStatus
	switch action {
	case models.RequestActionAccept:
		if responderID != req.TargetUserID {
			return nil, models.NewAPIError(models.ErrCodeInvalidAction, "Only the target user can accept this request")
		}
		newStatus = models.RequestStatusAccepted
	case models.RequestActionDecline:
		if responderID != req.TargetUserID {
			return nil, models.NewAPIError(models.ErrCodeInvalidAction, "Only the target user can decline this request")
		}
		newStatus = models.RequestStatusDeclined
	case models.RequestActionCancel:
		if responderID != req.CreatedByID {
			return nil, models.NewAPIError(models.ErrCodeInvalidAction, "Only the creator can cancel this request")
		}
		newStatus = models.RequestStatusCancelled
	default:
		return nil, models.ErrInvalidAction
	}

	now := time.Now().UTC()
	err = tx.QueryRow(ctx, `
		UPDATE requests SET status = $2, resolved_at = $3, updated_at = $3
		WHERE id = $1
		RETURNING id, chat_id, type, status, created_by_id, target_user_id,
			title, description, amount, currency, payload_json, expires_at, resolved_at, created_at, updated_at
	`, requestID, newStatus, now).Scan(
		&req.ID, &req.ChatID, &req.Type, &req.Status, &req.CreatedByID, &req.TargetUserID,
		&req.Title, &req.Description, &req.Amount, &req.Currency, &req.PayloadJSON,
		&req.ExpiresAt, &req.ResolvedAt, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Create system message about the action
	sysMsg := fmt.Sprintf("Request \"%s\" was %s", req.Title, string(newStatus))
	if note != nil && *note != "" {
		sysMsg += fmt.Sprintf(": %s", *note)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, type, body, request_id, created_at)
		VALUES ($1, $2, $3, 'system', $4, $5, $6)
	`, uuid.New(), req.ChatID, responderID, sysMsg, req.ID, now)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `UPDATE chats SET last_message_at = $2 WHERE id = $1`, req.ChatID, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &req, nil
}

// ListRequests returns requests for a chat, lazy-expiring pending ones.
func (r *ChatRepository) ListRequests(ctx context.Context, chatID uuid.UUID, statusFilter *string) ([]models.RequestResponse, error) {
	// First lazy-expire any pending requests past their deadline
	_, _ = r.db.Pool.Exec(ctx, `
		UPDATE requests SET status = 'expired', updated_at = NOW()
		WHERE chat_id = $1 AND status = 'pending' AND expires_at <= NOW()
	`, chatID)

	query := `
		SELECT r.id, r.chat_id, r.type, r.status, r.created_by_id,
			(SELECT first_name || ' ' || last_name FROM users WHERE id = r.created_by_id) AS created_by_name,
			r.target_user_id, r.title, r.description, r.amount, r.currency,
			r.expires_at, r.created_at, r.updated_at, r.resolved_at
		FROM requests r
		WHERE r.chat_id = $1
	`
	args := []interface{}{chatID}

	if statusFilter != nil && *statusFilter != "" {
		query += ` AND r.status = $2`
		args = append(args, *statusFilter)
	}
	query += ` ORDER BY r.created_at DESC`

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.RequestResponse, 0)
	for rows.Next() {
		var resp models.RequestResponse
		var expiresAt, createdAt, updatedAt time.Time
		var resolvedAt *time.Time
		err := rows.Scan(
			&resp.ID, &resp.ChatID, &resp.Type, &resp.Status, &resp.CreatedByID,
			&resp.CreatedByName, &resp.TargetUserID, &resp.Title, &resp.Description,
			&resp.Amount, &resp.Currency,
			&expiresAt, &createdAt, &updatedAt, &resolvedAt,
		)
		if err != nil {
			return nil, err
		}
		resp.ExpiresAt = models.RFC3339Time(expiresAt)
		resp.CreatedAt = models.RFC3339Time(createdAt)
		resp.UpdatedAt = models.RFC3339Time(updatedAt)
		if resolvedAt != nil {
			t := models.RFC3339Time(*resolvedAt)
			resp.ResolvedAt = &t
		}
		resp.Attachments = make([]models.AttachmentResponse, 0)
		results = append(results, resp)
	}

	// Load attachments for requests
	if len(results) > 0 {
		reqIDs := make([]uuid.UUID, len(results))
		for i, r := range results {
			reqIDs[i] = r.ID
		}
		attMap, err := r.getAttachmentsForRequests(ctx, reqIDs)
		if err != nil {
			return nil, err
		}
		for i := range results {
			if atts, ok := attMap[results[i].ID]; ok {
				results[i].Attachments = atts
			}
		}
	}

	return results, nil
}

func (r *ChatRepository) getAttachmentsForRequests(ctx context.Context, reqIDs []uuid.UUID) (map[uuid.UUID][]models.AttachmentResponse, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, request_id, kind, filename, mime_type, file_size, file_url, created_at
		FROM attachments
		WHERE request_id = ANY($1)
		ORDER BY created_at ASC
	`, reqIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]models.AttachmentResponse)
	for rows.Next() {
		var a models.AttachmentResponse
		var reqID uuid.UUID
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &reqID, &a.Kind, &a.Filename, &a.MimeType, &a.FileSize, &a.FileURL, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = models.RFC3339Time(createdAt)
		result[reqID] = append(result[reqID], a)
	}
	return result, nil
}

// ListPendingActionsForUser returns pending requests targeting the given user across all chats.
// Results are ordered by expires_at ASC (most urgent first).
func (r *ChatRepository) ListPendingActionsForUser(ctx context.Context, userID uuid.UUID) ([]models.ActionItemResponse, error) {
	// Lazy-expire any past-deadline pending requests for this user
	_, _ = r.db.Pool.Exec(ctx, `
		UPDATE requests SET status = 'expired', updated_at = NOW()
		WHERE target_user_id = $1 AND status = 'pending' AND expires_at <= NOW()
	`, userID)

	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			req.id,
			req.type,
			req.status,
			c.id AS chat_id,
			c.car_id,
			car.title AS car_title,
			(SELECT cp2.file_url FROM car_photos cp2 WHERE cp2.car_id = c.car_id AND cp2.slot_type = 'cover_front' LIMIT 1) AS car_cover_photo_url,
			req.created_by_id,
			(SELECT first_name || ' ' || last_name FROM users WHERE id = req.created_by_id) AS created_by_name,
			req.target_user_id,
			(SELECT first_name || ' ' || last_name FROM users WHERE id = req.target_user_id) AS target_user_name,
			req.title,
			req.description,
			req.amount,
			req.currency,
			req.expires_at,
			req.created_at
		FROM requests req
		JOIN chats c ON c.id = req.chat_id
		JOIN cars car ON car.id = c.car_id
		WHERE req.target_user_id = $1
			AND req.status = 'pending'
			AND req.expires_at > NOW()
		ORDER BY req.expires_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list pending actions: %w", err)
	}
	defer rows.Close()

	items := make([]models.ActionItemResponse, 0)
	for rows.Next() {
		var item models.ActionItemResponse
		var expiresAt, createdAt time.Time
		err := rows.Scan(
			&item.RequestID, &item.RequestType, &item.RequestStatus,
			&item.ChatID, &item.CarID, &item.CarTitle, &item.CarCoverPhotoURL,
			&item.CreatedByID, &item.CreatedByName,
			&item.TargetUserID, &item.TargetUserName,
			&item.Title, &item.Description,
			&item.Amount, &item.Currency,
			&expiresAt, &createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan action item: %w", err)
		}
		item.ExpiresAt = models.RFC3339Time(expiresAt)
		item.CreatedAt = models.RFC3339Time(createdAt)
		items = append(items, item)
	}

	return items, nil
}

// GetChatDetails returns detailed chat info for the details screen.
func (r *ChatRepository) GetChatDetails(ctx context.Context, chatID, userID uuid.UUID) (*models.ChatDetailsResponse, error) {
	var resp models.ChatDetailsResponse
	resp.ChatID = chatID

	// Get chat + car info
	var chatCreatedAt time.Time
	var counterpartyID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		SELECT c.created_at,
			car.id, car.title, car.status, car.weekly_rent_price, car.currency,
			CASE WHEN c.driver_id = $2 THEN c.owner_id ELSE c.driver_id END AS counterparty_id
		FROM chats c
		JOIN cars car ON car.id = c.car_id
		WHERE c.id = $1
	`, chatID, userID).Scan(
		&chatCreatedAt,
		&resp.Car.ID, &resp.Car.Title, &resp.Car.Status,
		&resp.Car.WeeklyRentPrice, &resp.Car.Currency,
		&counterpartyID,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrChatNotFound
	}
	if err != nil {
		return nil, err
	}
	resp.CreatedAt = models.RFC3339Time(chatCreatedAt)

	// Car cover photo
	var coverURL *string
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT file_url FROM car_photos WHERE car_id = $1 AND slot_type = 'cover_front' LIMIT 1
	`, resp.Car.ID).Scan(&coverURL)
	resp.Car.CoverPhotoURL = coverURL

	// Counterparty info
	var memberSince time.Time
	err = r.db.Pool.QueryRow(ctx, `
		SELECT id, first_name || ' ' || last_name, profile_photo_url, role, created_at
		FROM users WHERE id = $1
	`, counterpartyID).Scan(
		&resp.Counterparty.ID, &resp.Counterparty.Name, &resp.Counterparty.AvatarURL,
		&resp.Counterparty.Role, &memberSince,
	)
	if err != nil {
		return nil, err
	}
	resp.Counterparty.MemberSince = models.RFC3339Time(memberSince)

	// Participant settings
	err = r.db.Pool.QueryRow(ctx, `
		SELECT auto_translate, notifications_muted FROM chat_participants
		WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID).Scan(&resp.AutoTranslateEnabled, &resp.NotificationsMuted)
	if err != nil {
		return nil, err
	}

	// Attachment counts
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM attachments WHERE chat_id = $1 AND kind = 'document'
	`, chatID).Scan(&resp.DocumentsCount)
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM attachments WHERE chat_id = $1 AND kind IN ('image', 'video')
	`, chatID).Scan(&resp.MediaCount)

	return &resp, nil
}

// UpdateChatSettings updates participant settings (auto_translate, muted).
func (r *ChatRepository) UpdateChatSettings(ctx context.Context, chatID, userID uuid.UUID, body *models.UpdateChatSettingsBody) error {
	if body.AutoTranslate != nil {
		_, err := r.db.Pool.Exec(ctx, `
			UPDATE chat_participants SET auto_translate = $3 WHERE chat_id = $1 AND user_id = $2
		`, chatID, userID, *body.AutoTranslate)
		if err != nil {
			return err
		}
	}
	if body.NotificationsMuted != nil {
		_, err := r.db.Pool.Exec(ctx, `
			UPDATE chat_participants SET notifications_muted = $3 WHERE chat_id = $1 AND user_id = $2
		`, chatID, userID, *body.NotificationsMuted)
		if err != nil {
			return err
		}
	}
	return nil
}

// ArchiveChat sets/clears the is_archived flag for a participant.
func (r *ChatRepository) ArchiveChat(ctx context.Context, chatID, userID uuid.UUID, archived bool) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE chat_participants SET is_archived = $3 WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID, archived)
	return err
}

// ListAttachments returns attachments for a chat filtered by kind.
func (r *ChatRepository) ListAttachments(ctx context.Context, chatID uuid.UUID, kind *string) ([]models.AttachmentResponse, error) {
	query := `
		SELECT id, kind, filename, mime_type, file_size, file_url, created_at
		FROM attachments WHERE chat_id = $1
	`
	args := []interface{}{chatID}
	if kind != nil && *kind != "" {
		if *kind == "media" {
			query += ` AND kind IN ('image', 'video')`
		} else {
			query += ` AND kind = $2`
			args = append(args, *kind)
		}
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.AttachmentResponse, 0)
	for rows.Next() {
		var a models.AttachmentResponse
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &a.Kind, &a.Filename, &a.MimeType, &a.FileSize, &a.FileURL, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = models.RFC3339Time(createdAt)
		results = append(results, a)
	}
	return results, nil
}

// CreateAttachment inserts a new attachment record.
func (r *ChatRepository) CreateAttachment(ctx context.Context, att *models.Attachment) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO attachments (id, chat_id, message_id, request_id, uploader_id, kind, filename, mime_type, file_size, file_path, file_url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, att.ID, att.ChatID, att.MessageID, att.RequestID, att.UploaderID,
		att.Kind, att.Filename, att.MimeType, att.FileSize, att.FilePath, att.FileURL, att.CreatedAt)
	return err
}

// GetUserProfileDetail returns a user's profile with role-specific fields.
func (r *ChatRepository) GetUserProfileDetail(ctx context.Context, userID uuid.UUID) (*models.UserProfileDetailResponse, error) {
	var resp models.UserProfileDetailResponse
	var memberSince time.Time
	var role models.Role
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, first_name, last_name, profile_photo_url, role, phone, created_at
		FROM users WHERE id = $1
	`, userID).Scan(
		&resp.ID, &resp.FirstName, &resp.LastName, &resp.AvatarURL, &role, &resp.Phone, &memberSince,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	resp.Role = role
	resp.MemberSince = models.RFC3339Time(memberSince)

	if role == models.RoleDriver {
		// Get driver's license URL
		var licURL *string
		_ = r.db.Pool.QueryRow(ctx, `
			SELECT '/uploads/documents/' || id || '/' || file_name
			FROM documents WHERE user_id = $1 AND type = 'drivers_license' LIMIT 1
		`, userID).Scan(&licURL)
		resp.LicenseDocURL = licURL

		trips := 0
		resp.TotalTrips = &trips
		years := 0
		resp.YearsLicensed = &years
	} else if role == models.RoleCarOwner {
		// Get total listings count
		var count int
		_ = r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM cars WHERE owner_id = $1`, userID).Scan(&count)
		resp.TotalListings = &count
	}

	return &resp, nil
}
