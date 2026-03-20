package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/models"
)

type LeaseRequestRepository struct {
	db *database.DB
}

func NewLeaseRequestRepository(db *database.DB) *LeaseRequestRepository {
	return &LeaseRequestRepository{db: db}
}

// CreateLeaseRequest creates a lease request, auto-creating a chat if needed.
// Returns the lease request with chat_id set.
func (r *LeaseRequestRepository) CreateLeaseRequest(ctx context.Context, lr *models.LeaseRequest) (*models.LeaseRequest, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	// Find or create chat for (listing, driver, owner)
	chatID := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO chats (id, car_id, driver_id, owner_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (car_id, driver_id, owner_id) DO NOTHING
	`, chatID, lr.ListingID, lr.DriverID, lr.OwnerID, now)
	if err != nil {
		return nil, fmt.Errorf("upsert chat: %w", err)
	}

	// Get the actual chat ID (may have existed)
	err = tx.QueryRow(ctx, `
		SELECT id FROM chats WHERE car_id = $1 AND driver_id = $2 AND owner_id = $3
	`, lr.ListingID, lr.DriverID, lr.OwnerID).Scan(&lr.ChatID)
	if err != nil {
		return nil, fmt.Errorf("select chat: %w", err)
	}

	// Ensure participants exist
	for _, uid := range []uuid.UUID{lr.DriverID, lr.OwnerID} {
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_participants (id, chat_id, user_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4)
			ON CONFLICT (chat_id, user_id) DO NOTHING
		`, uuid.New(), lr.ChatID, uid, now)
		if err != nil {
			return nil, fmt.Errorf("upsert participant %s: %w", uid, err)
		}
	}

	// Insert lease request
	lr.ID = uuid.New()
	lr.Status = models.LeaseStatusRequested
	lr.CreatedAt = now
	lr.UpdatedAt = now
	lr.ExpiresAt = now.Add(24 * time.Hour)

	err = tx.QueryRow(ctx, `
		INSERT INTO lease_requests (id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)
		RETURNING id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at
	`, lr.ID, lr.ChatID, lr.ListingID, lr.OwnerID, lr.DriverID, lr.Status,
		lr.WeeklyPrice, lr.Currency, lr.Weeks, lr.Message, lr.ExpiresAt, now,
	).Scan(
		&lr.ID, &lr.ChatID, &lr.ListingID, &lr.OwnerID, &lr.DriverID,
		&lr.Status, &lr.WeeklyPrice, &lr.Currency, &lr.Weeks, &lr.Message,
		&lr.ExpiresAt, &lr.CreatedAt, &lr.UpdatedAt,
	)
	if err != nil {
		// Check for unique constraint violation (duplicate active request)
		if isDuplicateKeyError(err) {
			return nil, models.ErrDuplicateLeaseReq
		}
		return nil, fmt.Errorf("insert lease request: %w", err)
	}

	// Create a system message in the chat
	_, err = tx.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, type, body, created_at)
		VALUES ($1, $2, $3, 'system', $4, $5)
	`, uuid.New(), lr.ChatID, lr.DriverID,
		fmt.Sprintf("New lease request: %d week(s) at %s %.2f/week", lr.Weeks, lr.Currency, lr.WeeklyPrice),
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert system message: %w", err)
	}

	// Update chat timestamps
	_, err = tx.Exec(ctx, `
		UPDATE chats SET last_message_at = $2, last_request_at = $2, updated_at = $2 WHERE id = $1
	`, lr.ChatID, now)
	if err != nil {
		return nil, fmt.Errorf("update chat timestamps: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return lr, nil
}

// GetByID returns a lease request by its ID.
func (r *LeaseRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.LeaseRequest, error) {
	var lr models.LeaseRequest
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at
		FROM lease_requests WHERE id = $1
	`, id).Scan(
		&lr.ID, &lr.ChatID, &lr.ListingID, &lr.OwnerID, &lr.DriverID,
		&lr.Status, &lr.WeeklyPrice, &lr.Currency, &lr.Weeks, &lr.Message,
		&lr.ExpiresAt, &lr.CreatedAt, &lr.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrLeaseRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &lr, nil
}

// ListForChat returns all lease requests for a given chat, most recent first.
func (r *LeaseRequestRepository) ListForChat(ctx context.Context, chatID uuid.UUID) ([]models.LeaseRequestResponse, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			lr.id, lr.chat_id, lr.listing_id, lr.owner_id, lr.driver_id, lr.status,
			lr.weekly_price, lr.currency, lr.weeks, lr.message, lr.expires_at, lr.created_at, lr.updated_at,
			(SELECT first_name || ' ' || last_name FROM users WHERE id = lr.driver_id) AS driver_name,
			(SELECT first_name || ' ' || last_name FROM users WHERE id = lr.owner_id) AS owner_name,
			(SELECT title FROM cars WHERE id = lr.listing_id) AS car_title,
			p.id AS payment_id, p.payment_intent_id, p.amount AS payment_amount,
			p.platform_fee_amount, p.currency AS payment_currency, p.status AS payment_status
		FROM lease_requests lr
		LEFT JOIN payments p ON p.lease_request_id = lr.id
		WHERE lr.chat_id = $1
		ORDER BY lr.created_at DESC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("list lease requests: %w", err)
	}
	defer rows.Close()

	results := make([]models.LeaseRequestResponse, 0)
	for rows.Next() {
		var resp models.LeaseRequestResponse
		var expiresAt, createdAt, updatedAt time.Time
		var message *string
		// Payment fields (nullable from LEFT JOIN)
		var paymentID *uuid.UUID
		var paymentIntentID *string
		var paymentAmount *int64
		var platformFee *int64
		var paymentCurrency *string
		var paymentStatus *models.PaymentStatus

		err := rows.Scan(
			&resp.ID, &resp.ChatID, &resp.ListingID, &resp.OwnerID, &resp.DriverID, &resp.Status,
			&resp.WeeklyPrice, &resp.Currency, &resp.Weeks, &message, &expiresAt, &createdAt, &updatedAt,
			&resp.DriverName, &resp.OwnerName, &resp.CarTitle,
			&paymentID, &paymentIntentID, &paymentAmount,
			&platformFee, &paymentCurrency, &paymentStatus,
		)
		if err != nil {
			return nil, fmt.Errorf("scan lease request: %w", err)
		}
		resp.Message = message
		resp.TotalAmount = resp.WeeklyPrice * float64(resp.Weeks)
		resp.ExpiresAt = models.RFC3339Time(expiresAt)
		resp.CreatedAt = models.RFC3339Time(createdAt)
		resp.UpdatedAt = models.RFC3339Time(updatedAt)

		if paymentID != nil {
			resp.Payment = &models.PaymentSummary{
				ID:                *paymentID,
				PaymentIntentID:   paymentIntentID,
				Amount:            *paymentAmount,
				PlatformFeeAmount: *platformFee,
				Currency:          *paymentCurrency,
				Status:            *paymentStatus,
			}
		}

		results = append(results, resp)
	}

	return results, nil
}

// AcceptLeaseRequest transitions a lease request from requested → accepted (owner only).
func (r *LeaseRequestRepository) AcceptLeaseRequest(ctx context.Context, id, ownerID uuid.UUID) (*models.LeaseRequest, error) {
	return r.updateStatus(ctx, id, ownerID, models.LeaseStatusRequested, models.LeaseStatusAccepted, "owner")
}

// DeclineLeaseRequest transitions a lease request from requested → declined (owner only).
func (r *LeaseRequestRepository) DeclineLeaseRequest(ctx context.Context, id, ownerID uuid.UUID) (*models.LeaseRequest, error) {
	return r.updateStatus(ctx, id, ownerID, models.LeaseStatusRequested, models.LeaseStatusDeclined, "owner")
}

// CancelLeaseRequest transitions a lease request from requested → cancelled (driver only).
func (r *LeaseRequestRepository) CancelLeaseRequest(ctx context.Context, id, driverID uuid.UUID) (*models.LeaseRequest, error) {
	return r.updateStatus(ctx, id, driverID, models.LeaseStatusRequested, models.LeaseStatusCancelled, "driver")
}

// SetPaymentPending transitions accepted → payment_pending.
func (r *LeaseRequestRepository) SetPaymentPending(ctx context.Context, id uuid.UUID) (*models.LeaseRequest, error) {
	var lr models.LeaseRequest
	err := r.db.Pool.QueryRow(ctx, `
		UPDATE lease_requests SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'accepted'
		RETURNING id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at
	`, id, models.LeaseStatusPaymentPending).Scan(
		&lr.ID, &lr.ChatID, &lr.ListingID, &lr.OwnerID, &lr.DriverID,
		&lr.Status, &lr.WeeklyPrice, &lr.Currency, &lr.Weeks, &lr.Message,
		&lr.ExpiresAt, &lr.CreatedAt, &lr.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrInvalidLeaseAction
	}
	if err != nil {
		return nil, err
	}
	return &lr, nil
}

// SetPaid transitions accepted/payment_pending → paid.
// Accepts both statuses because the webhook may arrive before SetPaymentPending completes.
func (r *LeaseRequestRepository) SetPaid(ctx context.Context, id uuid.UUID) (*models.LeaseRequest, error) {
	var lr models.LeaseRequest
	err := r.db.Pool.QueryRow(ctx, `
		UPDATE lease_requests SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status IN ('accepted', 'payment_pending')
		RETURNING id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at
	`, id, models.LeaseStatusPaid).Scan(
		&lr.ID, &lr.ChatID, &lr.ListingID, &lr.OwnerID, &lr.DriverID,
		&lr.Status, &lr.WeeklyPrice, &lr.Currency, &lr.Weeks, &lr.Message,
		&lr.ExpiresAt, &lr.CreatedAt, &lr.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrInvalidLeaseAction
	}
	if err != nil {
		return nil, err
	}
	return &lr, nil
}

// updateStatus is a generic state transition helper.
func (r *LeaseRequestRepository) updateStatus(ctx context.Context, id, actorID uuid.UUID, fromStatus, toStatus models.LeaseRequestStatus, role string) (*models.LeaseRequest, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock and validate
	var lr models.LeaseRequest
	err = tx.QueryRow(ctx, `
		SELECT id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at
		FROM lease_requests WHERE id = $1 FOR UPDATE
	`, id).Scan(
		&lr.ID, &lr.ChatID, &lr.ListingID, &lr.OwnerID, &lr.DriverID,
		&lr.Status, &lr.WeeklyPrice, &lr.Currency, &lr.Weeks, &lr.Message,
		&lr.ExpiresAt, &lr.CreatedAt, &lr.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrLeaseRequestNotFound
	}
	if err != nil {
		return nil, err
	}

	// Validate actor
	switch role {
	case "owner":
		if actorID != lr.OwnerID {
			return nil, models.NewAPIError(models.ErrCodeInvalidLeaseAction, "Only the owner can perform this action")
		}
	case "driver":
		if actorID != lr.DriverID {
			return nil, models.NewAPIError(models.ErrCodeInvalidLeaseAction, "Only the driver can perform this action")
		}
	}

	// Validate status transition
	if lr.Status != fromStatus {
		return nil, models.ErrInvalidLeaseAction
	}

	now := time.Now().UTC()
	err = tx.QueryRow(ctx, `
		UPDATE lease_requests SET status = $2, updated_at = $3
		WHERE id = $1
		RETURNING id, chat_id, listing_id, owner_id, driver_id, status, weekly_price, currency, weeks, message, expires_at, created_at, updated_at
	`, id, toStatus, now).Scan(
		&lr.ID, &lr.ChatID, &lr.ListingID, &lr.OwnerID, &lr.DriverID,
		&lr.Status, &lr.WeeklyPrice, &lr.Currency, &lr.Weeks, &lr.Message,
		&lr.ExpiresAt, &lr.CreatedAt, &lr.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// System message
	sysMsg := fmt.Sprintf("Lease request %s", string(toStatus))
	_, err = tx.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, type, body, created_at)
		VALUES ($1, $2, $3, 'system', $4, $5)
	`, uuid.New(), lr.ChatID, actorID, sysMsg, now)
	if err != nil {
		return nil, fmt.Errorf("insert system message: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE chats SET last_message_at = $2, updated_at = $2 WHERE id = $1`, lr.ChatID, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &lr, nil
}

// --- Today Actions ---

// ListTodayActionsForOwner returns pending lease requests where the user is the owner.
// Also lazy-expires overdue requests. Results ordered by expires_at ASC (most urgent first).
func (r *LeaseRequestRepository) ListTodayActionsForOwner(ctx context.Context, ownerID uuid.UUID) ([]models.TodayAction, error) {
	// Lazy-expire overdue lease requests
	_, _ = r.db.Pool.Exec(ctx, `
		UPDATE lease_requests SET status = 'expired', updated_at = NOW()
		WHERE owner_id = $1 AND status = 'requested' AND expires_at <= NOW()
	`, ownerID)

	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			lr.id,
			lr.weekly_price, lr.currency, lr.weeks,
			c.car_id,
			(SELECT title FROM cars WHERE id = c.car_id) AS car_title,
			lr.chat_id,
			lr.driver_id,
			(SELECT first_name || ' ' || last_name FROM users WHERE id = lr.driver_id) AS driver_name,
			lr.status,
			lr.created_at,
			lr.expires_at
		FROM lease_requests lr
		JOIN chats c ON c.id = lr.chat_id
		WHERE lr.owner_id = $1
			AND lr.status = 'requested'
			AND lr.expires_at > NOW()
		ORDER BY lr.expires_at ASC
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list today actions: %w", err)
	}
	defer rows.Close()

	actions := make([]models.TodayAction, 0)
	for rows.Next() {
		var a models.TodayAction
		var weeklyPrice float64
		var currency string
		var weeks int
		var createdAt, expiresAt time.Time

		err := rows.Scan(
			&a.ID,
			&weeklyPrice, &currency, &weeks,
			&a.CarID, &a.CarTitle,
			&a.ChatID,
			&a.CounterpartyID, &a.CounterpartyName,
			&a.Status,
			&createdAt, &expiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan today action: %w", err)
		}

		a.Type = models.TodayActionLeaseRequest
		a.Title = "Approve lease request"
		a.Body = fmt.Sprintf("%s wants to rent your %s for %d week(s) at %s %.0f/week",
			a.CounterpartyName, a.CarTitle, weeks, currency, weeklyPrice)
		a.PrimaryAction = "approve"
		a.SecondaryAction = "decline"
		a.CreatedAt = models.RFC3339Time(createdAt)
		a.ExpiresAt = models.RFC3339Time(expiresAt)

		actions = append(actions, a)
	}

	return actions, nil
}

// HasUnreadActions checks if any actions were created after lastSeenAt.
func (r *LeaseRequestRepository) HasUnreadActions(ctx context.Context, ownerID uuid.UUID, lastSeenAt time.Time) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM lease_requests
			WHERE owner_id = $1 AND status = 'requested' AND expires_at > NOW() AND created_at > $2
		)
	`, ownerID, lastSeenAt).Scan(&exists)
	return exists, err
}

// --- Payment repository methods ---

// CreatePayment creates a payment record for a lease request.
func (r *LeaseRequestRepository) CreatePayment(ctx context.Context, p *models.Payment) (*models.Payment, error) {
	p.ID = uuid.New()
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO payments (id, lease_request_id, provider, stripe_customer_id, payment_intent_id, payment_intent_client_secret, amount, currency, platform_fee_amount, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
		RETURNING id, lease_request_id, provider, stripe_customer_id, payment_intent_id, payment_intent_client_secret, amount, currency, platform_fee_amount, status, created_at, updated_at
	`, p.ID, p.LeaseRequestID, p.Provider, p.StripeCustomerID, p.PaymentIntentID, p.ClientSecret,
		p.Amount, p.Currency, p.PlatformFeeAmount, p.Status, now,
	).Scan(
		&p.ID, &p.LeaseRequestID, &p.Provider, &p.StripeCustomerID, &p.PaymentIntentID, &p.ClientSecret,
		&p.Amount, &p.Currency, &p.PlatformFeeAmount, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, models.ErrPaymentAlreadyExists
		}
		return nil, fmt.Errorf("insert payment: %w", err)
	}
	return p, nil
}

// GetPaymentByLeaseRequestID returns the payment for a lease request.
func (r *LeaseRequestRepository) GetPaymentByLeaseRequestID(ctx context.Context, leaseRequestID uuid.UUID) (*models.Payment, error) {
	var p models.Payment
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, lease_request_id, provider, stripe_customer_id, payment_intent_id, payment_intent_client_secret, amount, currency, platform_fee_amount, status, created_at, updated_at
		FROM payments WHERE lease_request_id = $1
	`, leaseRequestID).Scan(
		&p.ID, &p.LeaseRequestID, &p.Provider, &p.StripeCustomerID, &p.PaymentIntentID, &p.ClientSecret,
		&p.Amount, &p.Currency, &p.PlatformFeeAmount, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil // No payment yet — not an error
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPaymentByIntentID returns a payment by its Stripe PaymentIntent ID (for webhook handling).
func (r *LeaseRequestRepository) GetPaymentByIntentID(ctx context.Context, intentID string) (*models.Payment, error) {
	var p models.Payment
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, lease_request_id, provider, stripe_customer_id, payment_intent_id, payment_intent_client_secret, amount, currency, platform_fee_amount, status, created_at, updated_at
		FROM payments WHERE payment_intent_id = $1
	`, intentID).Scan(
		&p.ID, &p.LeaseRequestID, &p.Provider, &p.StripeCustomerID, &p.PaymentIntentID, &p.ClientSecret,
		&p.Amount, &p.Currency, &p.PlatformFeeAmount, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, models.ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePaymentStatus updates a payment's status (idempotent).
func (r *LeaseRequestRepository) UpdatePaymentStatus(ctx context.Context, paymentID uuid.UUID, status models.PaymentStatus) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE payments SET status = $2, updated_at = NOW() WHERE id = $1
	`, paymentID, status)
	return err
}

// isDuplicateKeyError checks for Postgres unique constraint violations.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	return fmt.Sprintf("%v", err) != "" &&
		(contains(err.Error(), "duplicate key") || contains(err.Error(), "23505"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
