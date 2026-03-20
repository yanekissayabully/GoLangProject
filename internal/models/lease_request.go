package models

import (
	"time"

	"github.com/google/uuid"
)

// LeaseRequestStatus represents the lifecycle of a lease request
type LeaseRequestStatus string

const (
	LeaseStatusRequested      LeaseRequestStatus = "requested"
	LeaseStatusAccepted       LeaseRequestStatus = "accepted"
	LeaseStatusDeclined       LeaseRequestStatus = "declined"
	LeaseStatusCancelled      LeaseRequestStatus = "cancelled"
	LeaseStatusPaymentPending LeaseRequestStatus = "payment_pending"
	LeaseStatusPaid           LeaseRequestStatus = "paid"
	LeaseStatusExpired        LeaseRequestStatus = "expired"
)

// PaymentStatus mirrors Stripe PaymentIntent statuses
type PaymentStatus string

const (
	PaymentStatusRequiresPaymentMethod PaymentStatus = "requires_payment_method"
	PaymentStatusRequiresConfirmation  PaymentStatus = "requires_confirmation"
	PaymentStatusProcessing            PaymentStatus = "processing"
	PaymentStatusSucceeded             PaymentStatus = "succeeded"
	PaymentStatusCanceled              PaymentStatus = "canceled"
	PaymentStatusFailed                PaymentStatus = "failed"
)

// LeaseRequest represents a driver's request to lease a car listing
type LeaseRequest struct {
	ID          uuid.UUID          `json:"id"`
	ChatID      uuid.UUID          `json:"chat_id"`
	ListingID   uuid.UUID          `json:"listing_id"`
	OwnerID     uuid.UUID          `json:"owner_id"`
	DriverID    uuid.UUID          `json:"driver_id"`
	Status      LeaseRequestStatus `json:"status"`
	WeeklyPrice float64            `json:"weekly_price"`
	Currency    string             `json:"currency"`
	Weeks       int                `json:"weeks"`
	Message     *string            `json:"message,omitempty"`
	ExpiresAt   time.Time          `json:"expires_at"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// TotalAmountCents returns the total amount in smallest currency unit (cents)
func (lr *LeaseRequest) TotalAmountCents() int64 {
	return int64(lr.WeeklyPrice * float64(lr.Weeks) * 100)
}

// Payment represents a Stripe payment linked to a lease request
type Payment struct {
	ID                uuid.UUID     `json:"id"`
	LeaseRequestID    uuid.UUID     `json:"lease_request_id"`
	Provider          string        `json:"provider"`
	StripeCustomerID  *string       `json:"stripe_customer_id,omitempty"`
	PaymentIntentID   *string       `json:"payment_intent_id,omitempty"`
	ClientSecret      *string       `json:"-"` // never serialized; sent to client via PaymentIntentResponse only
	Amount            int64         `json:"amount"`             // in cents
	Currency          string        `json:"currency"`
	PlatformFeeAmount int64         `json:"platform_fee_amount"` // in cents
	Status            PaymentStatus `json:"status"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

// Error codes for lease request operations
const (
	ErrCodeLeaseRequestNotFound = "LEASE_REQUEST_NOT_FOUND"
	ErrCodeDuplicateLeaseReq    = "DUPLICATE_LEASE_REQUEST"
	ErrCodeCannotLeaseOwnCar    = "CANNOT_LEASE_OWN_CAR"
	ErrCodeCarNotForRent        = "CAR_NOT_FOR_RENT"
	ErrCodePaymentNotFound      = "PAYMENT_NOT_FOUND"
	ErrCodePaymentAlreadyExists = "PAYMENT_ALREADY_EXISTS"
	ErrCodeInvalidLeaseAction   = "INVALID_LEASE_ACTION"
)

var (
	ErrLeaseRequestNotFound = &APIError{Code: ErrCodeLeaseRequestNotFound, Message: "Lease request not found"}
	ErrDuplicateLeaseReq    = &APIError{Code: ErrCodeDuplicateLeaseReq, Message: "You already have an active lease request for this listing"}
	ErrCannotLeaseOwnCar    = &APIError{Code: ErrCodeCannotLeaseOwnCar, Message: "You cannot request a lease on your own car"}
	ErrCarNotForRent        = &APIError{Code: ErrCodeCarNotForRent, Message: "This car is not available for rent"}
	ErrPaymentNotFound      = &APIError{Code: ErrCodePaymentNotFound, Message: "Payment not found"}
	ErrPaymentAlreadyExists = &APIError{Code: ErrCodePaymentAlreadyExists, Message: "Payment already exists for this lease request"}
	ErrInvalidLeaseAction   = &APIError{Code: ErrCodeInvalidLeaseAction, Message: "Invalid action for the current lease request status"}
)

// --- API request types ---

type CreateLeaseRequestBody struct {
	Weeks   *int    `json:"weeks,omitempty"`
	Message *string `json:"message,omitempty"`
}

// --- API response types ---

type LeaseRequestResponse struct {
	ID          uuid.UUID          `json:"id"`
	ChatID      uuid.UUID          `json:"chat_id"`
	ListingID   uuid.UUID          `json:"listing_id"`
	OwnerID     uuid.UUID          `json:"owner_id"`
	DriverID    uuid.UUID          `json:"driver_id"`
	DriverName  string             `json:"driver_name"`
	OwnerName   string             `json:"owner_name"`
	Status      LeaseRequestStatus `json:"status"`
	WeeklyPrice float64            `json:"weekly_price"`
	TotalAmount float64            `json:"total_amount"`
	Currency    string             `json:"currency"`
	Weeks       int                `json:"weeks"`
	Message     *string            `json:"message,omitempty"`
	CarTitle    string             `json:"car_title"`
	Payment     *PaymentSummary    `json:"payment,omitempty"`
	ExpiresAt   RFC3339Time        `json:"expires_at"`
	CreatedAt   RFC3339Time        `json:"created_at"`
	UpdatedAt   RFC3339Time        `json:"updated_at"`
}

type PaymentSummary struct {
	ID                uuid.UUID     `json:"id"`
	PaymentIntentID   *string       `json:"payment_intent_id,omitempty"`
	Amount            int64         `json:"amount"`
	PlatformFeeAmount int64         `json:"platform_fee_amount"`
	Currency          string        `json:"currency"`
	Status            PaymentStatus `json:"status"`
}

type LeaseRequestsListResponse struct {
	LeaseRequests []LeaseRequestResponse `json:"lease_requests"`
}

type CreateLeaseRequestResponse struct {
	ChatID       uuid.UUID            `json:"chat_id"`
	LeaseRequest LeaseRequestResponse `json:"lease_request"`
}

type PaymentIntentResponse struct {
	PaymentIntentClientSecret string `json:"payment_intent_client_secret"`
	PaymentIntentID           string `json:"payment_intent_id"`
	PublishableKey             string `json:"publishable_key"`
	CustomerID                string `json:"customer_id,omitempty"`
	EphemeralKeySecret        string `json:"ephemeral_key_secret,omitempty"`
	Amount                    int64  `json:"amount"`
	Currency                  string `json:"currency"`
}
