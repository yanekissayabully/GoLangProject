package models

import "github.com/google/uuid"

// TodayActionType distinguishes the source of an action.
type TodayActionType string

const (
	TodayActionLeaseRequest TodayActionType = "lease_request"
	// Future: TodayActionChatRequest TodayActionType = "chat_request"
)

// TodayAction is a single item in the owner/driver's Today feed.
type TodayAction struct {
	ID               uuid.UUID       `json:"id"`
	Type             TodayActionType `json:"type"`
	Title            string          `json:"title"`
	Body             string          `json:"body"`
	CarID            uuid.UUID       `json:"car_id"`
	CarTitle         string          `json:"car_title"`
	ChatID           uuid.UUID       `json:"chat_id"`
	CounterpartyID   uuid.UUID       `json:"counterparty_id"`
	CounterpartyName string          `json:"counterparty_name"`
	Status           string          `json:"status"`
	CreatedAt        RFC3339Time     `json:"created_at"`
	ExpiresAt        RFC3339Time     `json:"expires_at"`
	PrimaryAction    string          `json:"primary_action"`
	SecondaryAction  string          `json:"secondary_action"`
}

// TodayActionsResponse is the API response for GET /today/actions.
type TodayActionsResponse struct {
	Actions          []TodayAction `json:"actions"`
	HasUnreadActions bool          `json:"has_unread_actions"`
}
