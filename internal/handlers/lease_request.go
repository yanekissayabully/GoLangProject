package handlers

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/drivebai/backend/internal/httputil"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
	stripeService "github.com/drivebai/backend/internal/stripe"
	"github.com/drivebai/backend/internal/ws"
)

type LeaseRequestHandler struct {
	leaseRepo *repository.LeaseRequestRepository
	carRepo   *repository.CarRepository
	userRepo  *repository.UserRepository
	chatRepo  *repository.ChatRepository
	stripe    *stripeService.Service
	wsHub     *ws.Hub
	logger    *slog.Logger
}

func NewLeaseRequestHandler(
	leaseRepo *repository.LeaseRequestRepository,
	carRepo *repository.CarRepository,
	userRepo *repository.UserRepository,
	chatRepo *repository.ChatRepository,
	stripe *stripeService.Service,
	wsHub *ws.Hub,
	logger *slog.Logger,
) *LeaseRequestHandler {
	return &LeaseRequestHandler{
		leaseRepo: leaseRepo,
		carRepo:   carRepo,
		userRepo:  userRepo,
		chatRepo:  chatRepo,
		stripe:    stripe,
		wsHub:     wsHub,
		logger:    logger,
	}
}

// CreateLeaseRequest handles POST /api/v1/listings/{listingId}/lease-requests
func (h *LeaseRequestHandler) CreateLeaseRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	listingID, err := uuid.Parse(chi.URLParam(r, "listingId"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid listing ID"))
		return
	}

	var body models.CreateLeaseRequestBody
	if err := httputil.DecodeJSON(r, &body); err != nil {
		// Body is optional, allow empty
		body = models.CreateLeaseRequestBody{}
	}

	// Fetch the car listing
	car, err := h.carRepo.GetByID(r.Context(), listingID)
	if err != nil {
		h.logger.Error("get car for lease request", "error", err)
		httputil.WriteError(w, http.StatusNotFound, models.NewAPIError("CAR_NOT_FOUND", "Car listing not found"))
		return
	}

	// Validate: car must be for rent
	if !car.IsForRent || !car.WeeklyRentPrice.Valid {
		httputil.WriteError(w, http.StatusBadRequest, models.ErrCarNotForRent)
		return
	}

	// Validate: driver cannot request own car
	if userID == car.OwnerID {
		httputil.WriteError(w, http.StatusBadRequest, models.ErrCannotLeaseOwnCar)
		return
	}

	weeks := 1
	if body.Weeks != nil && *body.Weeks > 0 {
		weeks = *body.Weeks
	}

	lr := &models.LeaseRequest{
		ListingID:   listingID,
		OwnerID:     car.OwnerID,
		DriverID:    userID,
		WeeklyPrice: car.WeeklyRentPrice.Float64,
		Currency:    car.Currency,
		Weeks:       weeks,
		Message:     body.Message,
	}

	created, err := h.leaseRepo.CreateLeaseRequest(r.Context(), lr)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			httputil.WriteError(w, http.StatusConflict, apiErr)
		} else {
			h.logger.Error("create lease request", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	// Build response with names
	resp := h.buildLeaseRequestResponse(r, created, nil)

	httputil.WriteJSON(w, http.StatusCreated, models.CreateLeaseRequestResponse{
		ChatID:       created.ChatID,
		LeaseRequest: resp,
	})

	// Broadcast to owner via WebSocket
	h.wsHub.Broadcast(&ws.Event{
		Type:          "lease_request_created",
		Payload:       resp,
		TargetUserIDs: []uuid.UUID{created.OwnerID},
	})
}

// ListLeaseRequests handles GET /api/v1/chats/{chatId}/lease-requests
func (h *LeaseRequestHandler) ListLeaseRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	chatID, err := uuid.Parse(chi.URLParam(r, "chatId"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid chat ID"))
		return
	}

	// Verify participant
	isParticipant, err := h.chatRepo.IsParticipant(r.Context(), chatID, userID)
	if err != nil {
		h.logger.Error("check participant", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	if !isParticipant {
		httputil.WriteError(w, http.StatusForbidden, models.ErrNotParticipant)
		return
	}

	leaseRequests, err := h.leaseRepo.ListForChat(r.Context(), chatID)
	if err != nil {
		h.logger.Error("list lease requests", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, models.LeaseRequestsListResponse{
		LeaseRequests: leaseRequests,
	})
}

// AcceptLeaseRequest handles POST /api/v1/lease-requests/{id}/accept
func (h *LeaseRequestHandler) AcceptLeaseRequest(w http.ResponseWriter, r *http.Request) {
	h.handleLeaseAction(w, r, "accept")
}

// DeclineLeaseRequest handles POST /api/v1/lease-requests/{id}/decline
func (h *LeaseRequestHandler) DeclineLeaseRequest(w http.ResponseWriter, r *http.Request) {
	h.handleLeaseAction(w, r, "decline")
}

// CancelLeaseRequest handles POST /api/v1/lease-requests/{id}/cancel
func (h *LeaseRequestHandler) CancelLeaseRequest(w http.ResponseWriter, r *http.Request) {
	h.handleLeaseAction(w, r, "cancel")
}

func (h *LeaseRequestHandler) handleLeaseAction(w http.ResponseWriter, r *http.Request, action string) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	leaseID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid lease request ID"))
		return
	}

	var updated *models.LeaseRequest
	switch action {
	case "accept":
		updated, err = h.leaseRepo.AcceptLeaseRequest(r.Context(), leaseID, userID)
	case "decline":
		updated, err = h.leaseRepo.DeclineLeaseRequest(r.Context(), leaseID, userID)
	case "cancel":
		updated, err = h.leaseRepo.CancelLeaseRequest(r.Context(), leaseID, userID)
	}

	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			status := http.StatusBadRequest
			if apiErr.Code == models.ErrCodeLeaseRequestNotFound {
				status = http.StatusNotFound
			}
			httputil.WriteError(w, status, apiErr)
		} else {
			h.logger.Error("lease action", "action", action, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	resp := h.buildLeaseRequestResponse(r, updated, nil)
	httputil.WriteJSON(w, http.StatusOK, resp)

	// Broadcast to the other party
	otherUserID := updated.OwnerID
	if userID == updated.OwnerID {
		otherUserID = updated.DriverID
	}
	h.wsHub.Broadcast(&ws.Event{
		Type:          "lease_request_updated",
		Payload:       resp,
		TargetUserIDs: []uuid.UUID{otherUserID},
	})
}

// --- Payment endpoints ---

// CreatePaymentIntent handles POST /api/v1/lease-requests/{id}/payments/intent
func (h *LeaseRequestHandler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	leaseID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid lease request ID"))
		return
	}

	// Fetch lease request
	lr, err := h.leaseRepo.GetByID(r.Context(), leaseID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			httputil.WriteError(w, http.StatusNotFound, apiErr)
		} else {
			h.logger.Error("get lease request for payment", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	// Only the driver can pay
	if userID != lr.DriverID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "Only the driver can initiate payment"))
		return
	}

	// Must be in accepted status (or payment_pending if retrying)
	if lr.Status != models.LeaseStatusAccepted && lr.Status != models.LeaseStatusPaymentPending {
		httputil.WriteError(w, http.StatusBadRequest, models.NewAPIError(models.ErrCodeInvalidLeaseAction, "Lease request must be accepted before payment"))
		return
	}

	// Check if payment already exists (idempotent — return stored client_secret)
	existingPayment, err := h.leaseRepo.GetPaymentByLeaseRequestID(r.Context(), leaseID)
	if err != nil {
		h.logger.Error("check existing payment", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	if existingPayment != nil && existingPayment.PaymentIntentID != nil && existingPayment.ClientSecret != nil {
		// Already have a PaymentIntent with stored client_secret — return it
		customerID := ""
		ephemeralKeySecret := ""
		if existingPayment.StripeCustomerID != nil {
			customerID = *existingPayment.StripeCustomerID
			ek, ekErr := h.stripe.CreateEphemeralKey(customerID)
			if ekErr == nil {
				ephemeralKeySecret = ek.Secret
			}
		}

		h.logger.Info("returning existing payment intent", "lease_request_id", leaseID, "payment_intent_id", *existingPayment.PaymentIntentID)

		httputil.WriteJSON(w, http.StatusOK, models.PaymentIntentResponse{
			PaymentIntentClientSecret: *existingPayment.ClientSecret,
			PaymentIntentID:           *existingPayment.PaymentIntentID,
			PublishableKey:            h.stripe.PublishableKey(),
			CustomerID:               customerID,
			EphemeralKeySecret:        ephemeralKeySecret,
			Amount:                    existingPayment.Amount,
			Currency:                  existingPayment.Currency,
		})
		return
	}

	// Compute amount
	totalCents := lr.TotalAmountCents()
	platformFeeCents := h.stripe.PlatformFee(totalCents)

	// Get driver user for Stripe customer
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("get user for stripe", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Find or create Stripe customer
	customer, err := h.stripe.FindOrCreateCustomer(user.Email, user.FullName())
	if err != nil {
		h.logger.Error("stripe find/create customer", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.NewAPIError("STRIPE_ERROR", "Failed to create payment customer"))
		return
	}

	// Create ephemeral key
	ephemeralKey, err := h.stripe.CreateEphemeralKey(customer.ID)
	if err != nil {
		h.logger.Error("stripe create ephemeral key", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.NewAPIError("STRIPE_ERROR", "Failed to create ephemeral key"))
		return
	}

	// Create PaymentIntent (idempotency key = lease request ID)
	pi, err := h.stripe.CreatePaymentIntent(totalCents, lr.Currency, customer.ID, platformFeeCents, leaseID.String())
	if err != nil {
		h.logger.Error("stripe create payment intent", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, models.NewAPIError("STRIPE_ERROR", "Failed to create payment"))
		return
	}

	h.logger.Info("payment intent created",
		"lease_request_id", leaseID,
		"payment_intent_id", pi.ID,
		"amount_cents", totalCents,
		"currency", lr.Currency,
		"customer_id", customer.ID,
	)

	// Save payment record (including client_secret for retry)
	payment := &models.Payment{
		LeaseRequestID:    leaseID,
		Provider:          "stripe",
		StripeCustomerID:  &customer.ID,
		PaymentIntentID:   &pi.ID,
		ClientSecret:      &pi.ClientSecret,
		Amount:            totalCents,
		Currency:          lr.Currency,
		PlatformFeeAmount: platformFeeCents,
		Status:            models.PaymentStatusRequiresPaymentMethod,
	}

	_, err = h.leaseRepo.CreatePayment(r.Context(), payment)
	if err != nil {
		// If duplicate, that's OK — idempotent
		if apiErr := models.GetAPIError(err); apiErr != nil && apiErr.Code == models.ErrCodePaymentAlreadyExists {
			h.logger.Info("payment already exists, returning existing", "lease_request_id", leaseID)
		} else {
			h.logger.Error("save payment record", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
			return
		}
	}

	// Transition lease request to payment_pending
	if lr.Status == models.LeaseStatusAccepted {
		_, err = h.leaseRepo.SetPaymentPending(r.Context(), leaseID)
		if err != nil {
			h.logger.Warn("failed to set payment_pending", "error", err)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, models.PaymentIntentResponse{
		PaymentIntentClientSecret: pi.ClientSecret,
		PaymentIntentID:           pi.ID,
		PublishableKey:            h.stripe.PublishableKey(),
		CustomerID:               customer.ID,
		EphemeralKeySecret:        ephemeralKey.Secret,
		Amount:                    totalCents,
		Currency:                  lr.Currency,
	})
}

// SyncPaymentStatus handles POST /api/v1/lease-requests/{id}/payments/sync
// Fallback mechanism: queries Stripe for current PaymentIntent status and reconciles locally.
func (h *LeaseRequestHandler) SyncPaymentStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.GetUserID(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	leaseID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid lease request ID"))
		return
	}

	lr, err := h.leaseRepo.GetByID(r.Context(), leaseID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, models.ErrLeaseRequestNotFound)
		return
	}

	// Only participants can sync
	if userID != lr.DriverID && userID != lr.OwnerID {
		httputil.WriteError(w, http.StatusForbidden, models.NewAPIError("FORBIDDEN", "Not a participant"))
		return
	}

	// If already paid, return current state
	if lr.Status == models.LeaseStatusPaid {
		resp := h.buildLeaseRequestResponse(r, lr, nil)
		httputil.WriteJSON(w, http.StatusOK, resp)
		return
	}

	// Get local payment record
	payment, err := h.leaseRepo.GetPaymentByLeaseRequestID(r.Context(), leaseID)
	if err != nil || payment == nil || payment.PaymentIntentID == nil {
		resp := h.buildLeaseRequestResponse(r, lr, payment)
		httputil.WriteJSON(w, http.StatusOK, resp)
		return
	}

	// Query Stripe for current PI status
	pi, err := h.stripe.RetrievePaymentIntent(*payment.PaymentIntentID)
	if err != nil {
		h.logger.Error("sync: retrieve PI from Stripe", "error", err, "intent_id", *payment.PaymentIntentID)
		resp := h.buildLeaseRequestResponse(r, lr, payment)
		httputil.WriteJSON(w, http.StatusOK, resp)
		return
	}

	h.logger.Info("sync: stripe PI status", "intent_id", pi.ID, "stripe_status", pi.Status, "local_payment_status", payment.Status, "lease_status", lr.Status)

	// Map Stripe status → local PaymentStatus
	newStatus := mapStripeStatus(pi.Status, payment.Status)

	// Update payment status if changed
	if newStatus != payment.Status {
		if err := h.leaseRepo.UpdatePaymentStatus(r.Context(), payment.ID, newStatus); err != nil {
			h.logger.Error("sync: update payment status", "error", err)
		} else {
			payment.Status = newStatus
		}
	}

	// If payment succeeded, transition lease to paid
	if newStatus == models.PaymentStatusSucceeded && lr.Status != models.LeaseStatusPaid {
		updatedLR, err := h.leaseRepo.SetPaid(r.Context(), leaseID)
		if err != nil {
			h.logger.Warn("sync: set lease paid", "error", err, "lease_request_id", leaseID, "current_status", lr.Status)
		} else {
			lr = updatedLR
			h.logger.Info("sync: lease transitioned to paid", "lease_request_id", leaseID)
			// Broadcast the update
			resp := h.buildLeaseRequestResponse(r, lr, payment)
			h.wsHub.Broadcast(&ws.Event{
				Type:          "lease_request_updated",
				Payload:       resp,
				TargetUserIDs: []uuid.UUID{lr.DriverID, lr.OwnerID},
			})
		}
	}

	resp := h.buildLeaseRequestResponse(r, lr, payment)
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// mapStripeStatus converts a Stripe PaymentIntent status string to our PaymentStatus.
func mapStripeStatus(stripeStatus string, fallback models.PaymentStatus) models.PaymentStatus {
	switch stripeStatus {
	case "succeeded":
		return models.PaymentStatusSucceeded
	case "processing":
		return models.PaymentStatusProcessing
	case "requires_payment_method":
		return models.PaymentStatusRequiresPaymentMethod
	case "requires_confirmation":
		return models.PaymentStatusRequiresConfirmation
	case "canceled":
		return models.PaymentStatusCanceled
	default:
		return fallback
	}
}

// HandleWebhook handles POST /api/v1/stripe/webhook
func (h *LeaseRequestHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		h.logger.Error("read webhook body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	if sigHeader == "" {
		h.logger.Warn("webhook: missing Stripe-Signature header")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := h.stripe.VerifyWebhookSignature(payload, sigHeader)
	if err != nil {
		h.logger.Warn("webhook: signature verification failed", "error", err, "webhook_secret_set", h.stripe.WebhookSecret() != "")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventType, _ := event["type"].(string)
	dataObj, _ := event["data"].(map[string]interface{})
	obj, _ := dataObj["object"].(map[string]interface{})
	intentID, _ := obj["id"].(string)

	h.logger.Info("webhook: event received", "type", eventType, "intent_id", intentID, "verified", true)

	if intentID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch eventType {
	case "payment_intent.succeeded":
		h.handlePaymentSucceeded(r, intentID)
	case "payment_intent.payment_failed":
		h.handlePaymentFailed(r, intentID)
	case "payment_intent.canceled":
		h.handlePaymentCanceled(r, intentID)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *LeaseRequestHandler) handlePaymentSucceeded(r *http.Request, intentID string) {
	payment, err := h.leaseRepo.GetPaymentByIntentID(r.Context(), intentID)
	if err != nil {
		// Not found = PI was not created by us; ignore silently
		if apiErr := models.GetAPIError(err); apiErr != nil && apiErr.Code == models.ErrCodePaymentNotFound {
			h.logger.Info("webhook: ignoring unknown payment_intent", "intent_id", intentID)
		} else {
			h.logger.Error("webhook: get payment by intent", "event", "succeeded", "intent_id", intentID, "error", err)
		}
		return
	}

	// Idempotency: if already succeeded, skip
	if payment.Status == models.PaymentStatusSucceeded {
		h.logger.Info("webhook: payment already succeeded (idempotent skip)", "intent_id", intentID, "payment_id", payment.ID)
		return
	}

	// Update payment status
	if err := h.leaseRepo.UpdatePaymentStatus(r.Context(), payment.ID, models.PaymentStatusSucceeded); err != nil {
		h.logger.Error("webhook: update payment status", "event", "succeeded", "payment_id", payment.ID, "error", err)
		return
	}

	// Transition lease request to paid (accepts both accepted and payment_pending)
	lr, err := h.leaseRepo.SetPaid(r.Context(), payment.LeaseRequestID)
	if err != nil {
		// If already in a terminal state (paid/declined/cancelled), log as idempotent
		if apiErr := models.GetAPIError(err); apiErr != nil && apiErr.Code == models.ErrCodeInvalidLeaseAction {
			h.logger.Info("webhook: lease already in terminal state (idempotent skip)", "lease_request_id", payment.LeaseRequestID, "intent_id", intentID)
			// Still broadcast in case the client missed the first one
			lr, _ = h.leaseRepo.GetByID(r.Context(), payment.LeaseRequestID)
		} else {
			h.logger.Error("webhook: set lease paid", "lease_request_id", payment.LeaseRequestID, "error", err)
			return
		}
	}

	if lr == nil {
		return
	}

	h.logger.Info("payment succeeded", "lease_request_id", lr.ID, "payment_id", payment.ID, "intent_id", intentID)

	// Broadcast update to both parties
	resp := h.buildLeaseRequestResponse(r, lr, nil)
	h.wsHub.Broadcast(&ws.Event{
		Type:          "lease_request_updated",
		Payload:       resp,
		TargetUserIDs: []uuid.UUID{lr.DriverID, lr.OwnerID},
	})
}

func (h *LeaseRequestHandler) handlePaymentFailed(r *http.Request, intentID string) {
	payment, err := h.leaseRepo.GetPaymentByIntentID(r.Context(), intentID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil && apiErr.Code == models.ErrCodePaymentNotFound {
			h.logger.Info("webhook: ignoring unknown payment_intent", "intent_id", intentID)
		} else {
			h.logger.Error("webhook: get payment by intent", "event", "failed", "intent_id", intentID, "error", err)
		}
		return
	}

	// Idempotency: already in terminal state
	if payment.Status == models.PaymentStatusFailed || payment.Status == models.PaymentStatusSucceeded {
		h.logger.Info("webhook: payment already terminal (idempotent skip)", "event", "failed", "intent_id", intentID, "status", payment.Status)
		return
	}

	if err := h.leaseRepo.UpdatePaymentStatus(r.Context(), payment.ID, models.PaymentStatusFailed); err != nil {
		h.logger.Error("webhook: update payment status", "event", "failed", "payment_id", payment.ID, "error", err)
	}

	h.logger.Info("payment failed", "lease_request_id", payment.LeaseRequestID, "payment_id", payment.ID, "intent_id", intentID)
}

func (h *LeaseRequestHandler) handlePaymentCanceled(r *http.Request, intentID string) {
	payment, err := h.leaseRepo.GetPaymentByIntentID(r.Context(), intentID)
	if err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil && apiErr.Code == models.ErrCodePaymentNotFound {
			h.logger.Info("webhook: ignoring unknown payment_intent", "intent_id", intentID)
		} else {
			h.logger.Error("webhook: get payment by intent", "event", "canceled", "intent_id", intentID, "error", err)
		}
		return
	}

	// Idempotency: already in terminal state
	if payment.Status == models.PaymentStatusCanceled || payment.Status == models.PaymentStatusSucceeded {
		h.logger.Info("webhook: payment already terminal (idempotent skip)", "event", "canceled", "intent_id", intentID, "status", payment.Status)
		return
	}

	if err := h.leaseRepo.UpdatePaymentStatus(r.Context(), payment.ID, models.PaymentStatusCanceled); err != nil {
		h.logger.Error("webhook: update payment status", "event", "canceled", "payment_id", payment.ID, "error", err)
	}

	h.logger.Info("payment canceled", "lease_request_id", payment.LeaseRequestID, "payment_id", payment.ID, "intent_id", intentID)
}

// --- Helpers ---

func (h *LeaseRequestHandler) buildLeaseRequestResponse(r *http.Request, lr *models.LeaseRequest, payment *models.Payment) models.LeaseRequestResponse {
	resp := models.LeaseRequestResponse{
		ID:          lr.ID,
		ChatID:      lr.ChatID,
		ListingID:   lr.ListingID,
		OwnerID:     lr.OwnerID,
		DriverID:    lr.DriverID,
		Status:      lr.Status,
		WeeklyPrice: lr.WeeklyPrice,
		TotalAmount: lr.WeeklyPrice * float64(lr.Weeks),
		Currency:    lr.Currency,
		Weeks:       lr.Weeks,
		Message:     lr.Message,
		ExpiresAt:   models.RFC3339Time(lr.ExpiresAt),
		CreatedAt:   models.RFC3339Time(lr.CreatedAt),
		UpdatedAt:   models.RFC3339Time(lr.UpdatedAt),
	}

	// Look up names
	if driver, err := h.userRepo.GetByID(r.Context(), lr.DriverID); err == nil {
		resp.DriverName = driver.FullName()
	}
	if owner, err := h.userRepo.GetByID(r.Context(), lr.OwnerID); err == nil {
		resp.OwnerName = owner.FullName()
	}

	// Car title
	if car, err := h.carRepo.GetByID(r.Context(), lr.ListingID); err == nil {
		resp.CarTitle = car.Title
	}

	// Payment summary
	if payment != nil {
		resp.Payment = &models.PaymentSummary{
			ID:                payment.ID,
			PaymentIntentID:   payment.PaymentIntentID,
			Amount:            payment.Amount,
			PlatformFeeAmount: payment.PlatformFeeAmount,
			Currency:          payment.Currency,
			Status:            payment.Status,
		}
	} else {
		// Try to load payment
		if p, err := h.leaseRepo.GetPaymentByLeaseRequestID(r.Context(), lr.ID); err == nil && p != nil {
			resp.Payment = &models.PaymentSummary{
				ID:                p.ID,
				PaymentIntentID:   p.PaymentIntentID,
				Amount:            p.Amount,
				PlatformFeeAmount: p.PlatformFeeAmount,
				Currency:          p.Currency,
				Status:            p.Status,
			}
		}
	}

	return resp
}
