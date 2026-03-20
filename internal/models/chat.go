package models

import (
	"time"

	"github.com/google/uuid"
)

// Chat enums

type MessageType string

const (
	MessageTypeText   MessageType = "text"
	MessageTypeSystem MessageType = "system"
)

type RequestType string

const (
	RequestTypeManualPayment  RequestType = "manual_payment"
	RequestTypeDelayedPayment RequestType = "delayed_payment"
	RequestTypeMechanicSvc    RequestType = "mechanic_service"
	RequestTypeAdditionalFee  RequestType = "additional_fee"
	RequestTypeGeneric        RequestType = "generic"
)

func (r RequestType) IsValid() bool {
	switch r {
	case RequestTypeManualPayment, RequestTypeDelayedPayment, RequestTypeMechanicSvc, RequestTypeAdditionalFee, RequestTypeGeneric:
		return true
	}
	return false
}

type RequestStatus string

const (
	RequestStatusPending   RequestStatus = "pending"
	RequestStatusAccepted  RequestStatus = "accepted"
	RequestStatusDeclined  RequestStatus = "declined"
	RequestStatusExpired   RequestStatus = "expired"
	RequestStatusCancelled RequestStatus = "cancelled"
)

type RequestAction string

const (
	RequestActionAccept  RequestAction = "accept"
	RequestActionDecline RequestAction = "decline"
	RequestActionCancel  RequestAction = "cancel"
)

func (a RequestAction) IsValid() bool {
	switch a {
	case RequestActionAccept, RequestActionDecline, RequestActionCancel:
		return true
	}
	return false
}

type AttachmentKind string

const (
	AttachmentKindImage    AttachmentKind = "image"
	AttachmentKindDocument AttachmentKind = "document"
	AttachmentKindVideo    AttachmentKind = "video"
)

func (k AttachmentKind) IsValid() bool {
	switch k {
	case AttachmentKindImage, AttachmentKindDocument, AttachmentKindVideo:
		return true
	}
	return false
}

// DB models

type Chat struct {
	ID            uuid.UUID  `json:"id"`
	CarID         uuid.UUID  `json:"car_id"`
	DriverID      uuid.UUID  `json:"driver_id"`
	OwnerID       uuid.UUID  `json:"owner_id"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	LastRequestAt *time.Time `json:"last_request_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ChatParticipant struct {
	ID                uuid.UUID `json:"id"`
	ChatID            uuid.UUID `json:"chat_id"`
	UserID            uuid.UUID `json:"user_id"`
	LastReadAt        time.Time `json:"last_read_at"`
	AutoTranslate     bool      `json:"auto_translate"`
	NotificationsMuted bool     `json:"notifications_muted"`
	IsArchived        bool      `json:"is_archived"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Message struct {
	ID              uuid.UUID   `json:"id"`
	ChatID          uuid.UUID   `json:"chat_id"`
	SenderID        uuid.UUID   `json:"sender_id"`
	Type            MessageType `json:"type"`
	Body            string      `json:"body"`
	ClientMessageID *uuid.UUID  `json:"client_message_id,omitempty"`
	RequestID       *uuid.UUID  `json:"request_id,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
}

type Request struct {
	ID           uuid.UUID     `json:"id"`
	ChatID       uuid.UUID     `json:"chat_id"`
	Type         RequestType   `json:"type"`
	Status       RequestStatus `json:"status"`
	CreatedByID  uuid.UUID     `json:"created_by_id"`
	TargetUserID uuid.UUID     `json:"target_user_id"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Amount       *float64      `json:"amount,omitempty"`
	Currency     string        `json:"currency"`
	PayloadJSON  string        `json:"payload_json"`
	ExpiresAt    time.Time     `json:"expires_at"`
	ResolvedAt   *time.Time    `json:"resolved_at,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

func (r *Request) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

func (r *Request) IsPending() bool {
	return r.Status == RequestStatusPending
}

type Attachment struct {
	ID         uuid.UUID      `json:"id"`
	ChatID     uuid.UUID      `json:"chat_id"`
	MessageID  *uuid.UUID     `json:"message_id,omitempty"`
	RequestID  *uuid.UUID     `json:"request_id,omitempty"`
	UploaderID uuid.UUID      `json:"uploader_id"`
	Kind       AttachmentKind `json:"kind"`
	Filename   string         `json:"filename"`
	MimeType   string         `json:"mime_type"`
	FileSize   int            `json:"file_size"`
	FilePath   string         `json:"-"`
	FileURL    string         `json:"file_url"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Error codes for chat operations
const (
	ErrCodeChatNotFound    = "CHAT_NOT_FOUND"
	ErrCodeNotParticipant  = "NOT_PARTICIPANT"
	ErrCodeRequestNotFound = "REQUEST_NOT_FOUND"
	ErrCodeRequestExpired  = "REQUEST_EXPIRED"
	ErrCodeInvalidAction   = "INVALID_ACTION"
	ErrCodeMessageNotFound = "MESSAGE_NOT_FOUND"
)

var (
	ErrChatNotFound    = &APIError{Code: ErrCodeChatNotFound, Message: "Chat not found"}
	ErrNotParticipant  = &APIError{Code: ErrCodeNotParticipant, Message: "You are not a participant in this chat"}
	ErrRequestNotFound = &APIError{Code: ErrCodeRequestNotFound, Message: "Request not found"}
	ErrRequestExpired  = &APIError{Code: ErrCodeRequestExpired, Message: "This request has expired"}
	ErrInvalidAction   = &APIError{Code: ErrCodeInvalidAction, Message: "Invalid action for this request"}
	ErrMessageNotFound = &APIError{Code: ErrCodeMessageNotFound, Message: "Message not found"}
)

// API response types

type ChatListItemResponse struct {
	ID                   uuid.UUID   `json:"id"`
	CarID                uuid.UUID   `json:"car_id"`
	CarTitle             string      `json:"car_title"`
	CarCoverPhotoURL     *string     `json:"car_cover_photo_url,omitempty"`
	CounterpartyID       uuid.UUID   `json:"counterparty_id"`
	CounterpartyName     string      `json:"counterparty_name"`
	CounterpartyAvatarURL *string    `json:"counterparty_avatar_url,omitempty"`
	LastMessage          *string     `json:"last_message,omitempty"`
	LastMessageAt        *RFC3339Time `json:"last_message_at,omitempty"`
	UnreadCount          int         `json:"unread_count"`
	OpenRequestsCount    int         `json:"open_requests_count"`
	IsArchived           bool        `json:"is_archived"`
}

type ChatsListResponse struct {
	Chats       []ChatListItemResponse `json:"chats"`
	TotalUnread int                    `json:"total_unread"`
}

type MessageResponse struct {
	ID              uuid.UUID      `json:"id"`
	ChatID          uuid.UUID      `json:"chat_id"`
	SenderID        uuid.UUID      `json:"sender_id"`
	SenderName      string         `json:"sender_name"`
	Type            MessageType    `json:"type"`
	Body            string         `json:"body"`
	ClientMessageID *uuid.UUID     `json:"client_message_id,omitempty"`
	RequestID       *uuid.UUID     `json:"request_id,omitempty"`
	Attachments     []AttachmentResponse `json:"attachments"`
	CreatedAt       RFC3339Time    `json:"created_at"`
}

type MessagesPageResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextCursor *string           `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

type RequestResponse struct {
	ID            uuid.UUID          `json:"id"`
	ChatID        uuid.UUID          `json:"chat_id"`
	Type          RequestType        `json:"type"`
	Status        RequestStatus      `json:"status"`
	CreatedByID   uuid.UUID          `json:"created_by_id"`
	CreatedByName string             `json:"created_by_name"`
	TargetUserID  uuid.UUID          `json:"target_user_id"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Amount        *float64           `json:"amount,omitempty"`
	Currency      string             `json:"currency"`
	Attachments   []AttachmentResponse `json:"attachments"`
	ExpiresAt     RFC3339Time        `json:"expires_at"`
	CreatedAt     RFC3339Time        `json:"created_at"`
	UpdatedAt     RFC3339Time        `json:"updated_at"`
	ResolvedAt    *RFC3339Time       `json:"resolved_at,omitempty"`
	ResponseNote  *string            `json:"response_note,omitempty"`
}

type RequestsListResponse struct {
	Requests []RequestResponse `json:"requests"`
}

type AttachmentResponse struct {
	ID        uuid.UUID      `json:"id"`
	Kind      AttachmentKind `json:"kind"`
	Filename  string         `json:"filename"`
	MimeType  string         `json:"mime_type"`
	FileSize  int            `json:"file_size"`
	FileURL   string         `json:"file_url"`
	CreatedAt RFC3339Time    `json:"created_at"`
}

type ChatDetailsResponse struct {
	ChatID               uuid.UUID             `json:"chat_id"`
	Car                  ChatCarInfoResponse    `json:"car"`
	Counterparty         ChatParticipantInfoResponse `json:"counterparty"`
	AutoTranslateEnabled bool                  `json:"auto_translation_enabled"`
	NotificationsMuted   bool                  `json:"notifications_muted"`
	DocumentsCount       int                   `json:"documents_count"`
	MediaCount           int                   `json:"media_count"`
	CreatedAt            RFC3339Time           `json:"created_at"`
}

type ChatCarInfoResponse struct {
	ID              uuid.UUID        `json:"id"`
	Title           string           `json:"title"`
	CoverPhotoURL   *string          `json:"cover_photo_url,omitempty"`
	Status          CarListingStatus `json:"status"`
	WeeklyRentPrice *float64         `json:"weekly_rent_price,omitempty"`
	Currency        string           `json:"currency"`
}

type ChatParticipantInfoResponse struct {
	ID              uuid.UUID   `json:"id"`
	Name            string      `json:"name"`
	AvatarURL       *string     `json:"avatar_url,omitempty"`
	Role            Role        `json:"role"`
	MemberSince     RFC3339Time `json:"member_since"`
}

type UserProfileDetailResponse struct {
	ID              uuid.UUID   `json:"id"`
	FirstName       string      `json:"first_name"`
	LastName        string      `json:"last_name"`
	AvatarURL       *string     `json:"avatar_url,omitempty"`
	Role            Role        `json:"role"`
	MemberSince     RFC3339Time `json:"member_since"`
	Phone           *string     `json:"phone,omitempty"`
	// Driver fields (visible to owners)
	LicenseDocURL   *string     `json:"license_document_url,omitempty"`
	TotalTrips      *int        `json:"total_trips,omitempty"`
	YearsLicensed   *int        `json:"years_licensed,omitempty"`
	// Owner fields (visible to drivers)
	MechanicName    *string     `json:"mechanic_name,omitempty"`
	MechanicPhone   *string     `json:"mechanic_phone,omitempty"`
	TotalListings   *int        `json:"total_listings,omitempty"`
}

// Action item for Today tab aggregation

type ActionItemResponse struct {
	RequestID        uuid.UUID     `json:"request_id"`
	RequestType      RequestType   `json:"request_type"`
	RequestStatus    RequestStatus `json:"request_status"`
	ChatID           uuid.UUID     `json:"chat_id"`
	CarID            uuid.UUID     `json:"car_id"`
	CarTitle         string        `json:"car_title"`
	CarCoverPhotoURL *string       `json:"car_cover_photo_url,omitempty"`
	CreatedByID      uuid.UUID     `json:"created_by_id"`
	CreatedByName    string        `json:"created_by_name"`
	TargetUserID     uuid.UUID     `json:"target_user_id"`
	TargetUserName   string        `json:"target_user_name"`
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	Amount           *float64      `json:"amount,omitempty"`
	Currency         string        `json:"currency"`
	ExpiresAt        RFC3339Time   `json:"expires_at"`
	CreatedAt        RFC3339Time   `json:"created_at"`
}

type ActionsListResponse struct {
	Actions []ActionItemResponse `json:"actions"`
}

// DefaultDeadline returns the default expiry duration per request type.
func DefaultDeadline(rt RequestType) time.Duration {
	switch rt {
	case RequestTypeManualPayment:
		return 24 * time.Hour
	case RequestTypeAdditionalFee:
		return 24 * time.Hour
	case RequestTypeMechanicSvc:
		return 48 * time.Hour
	case RequestTypeDelayedPayment:
		return 72 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// API request types

type FindOrCreateChatRequest struct {
	CarID    uuid.UUID `json:"car_id"`
	DriverID uuid.UUID `json:"driver_id"`
	OwnerID  uuid.UUID `json:"owner_id"`
}

type SendMessageRequestBody struct {
	Body            string    `json:"body"`
	ClientMessageID uuid.UUID `json:"client_message_id"`
}

type CreateRequestBody struct {
	Type        RequestType `json:"type"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Amount      *float64    `json:"amount,omitempty"`
	Currency    *string     `json:"currency,omitempty"`
	ExpiresAt   *time.Time  `json:"expires_at,omitempty"`
}

type RequestActionBody struct {
	Action RequestAction `json:"action"`
	Note   *string       `json:"note,omitempty"`
}

type UpdateChatSettingsBody struct {
	AutoTranslate      *bool `json:"auto_translation_enabled,omitempty"`
	NotificationsMuted *bool `json:"notifications_muted,omitempty"`
}

type ArchiveChatBody struct {
	Archived bool `json:"archived"`
}

type MarkReadBody struct {
	LastReadMessageID *uuid.UUID `json:"last_read_message_id,omitempty"`
}
