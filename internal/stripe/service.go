package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/stripe/stripe-go/v81/webhook"
)

// Service wraps Stripe API calls using direct HTTP (no SDK dependency).
type Service struct {
	secretKey      string
	publishableKey string
	webhookSecret  string
	feeBPS         int // platform fee in basis points
	logger         *slog.Logger
	httpClient     *http.Client
}

func NewService(secretKey, publishableKey, webhookSecret string, feeBPS int, logger *slog.Logger) *Service {
	return &Service{
		secretKey:      secretKey,
		publishableKey: publishableKey,
		webhookSecret:  webhookSecret,
		feeBPS:         feeBPS,
		logger:         logger,
		httpClient:     &http.Client{},
	}
}

func (s *Service) PublishableKey() string { return s.publishableKey }
func (s *Service) WebhookSecret() string  { return s.webhookSecret }

// PlatformFee calculates the platform fee in smallest currency unit.
func (s *Service) PlatformFee(amountCents int64) int64 {
	return amountCents * int64(s.feeBPS) / 10000
}

// --- Stripe API types ---

type Customer struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type EphemeralKey struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

type PaymentIntent struct {
	ID           string `json:"id"`
	ClientSecret string `json:"client_secret"`
	Status       string `json:"status"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
}

// --- Customer ---

// FindOrCreateCustomer finds a Stripe customer by email, or creates one.
func (s *Service) FindOrCreateCustomer(email, name string) (*Customer, error) {
	// Search for existing customer
	params := url.Values{}
	params.Set("query", fmt.Sprintf("email:'%s'", email))
	params.Set("limit", "1")

	body, err := s.apiGet("/v1/customers/search?" + params.Encode())
	if err != nil {
		s.logger.Warn("stripe customer search failed, creating new", "error", err)
	} else {
		var result struct {
			Data []Customer `json:"data"`
		}
		if json.Unmarshal(body, &result) == nil && len(result.Data) > 0 {
			return &result.Data[0], nil
		}
	}

	// Create new customer
	params = url.Values{}
	params.Set("email", email)
	params.Set("name", name)

	respBody, err := s.apiPost("/v1/customers", params)
	if err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}
	var cust Customer
	if err := json.Unmarshal(respBody, &cust); err != nil {
		return nil, fmt.Errorf("decode customer: %w", err)
	}
	return &cust, nil
}

// --- Ephemeral Key ---

// CreateEphemeralKey creates a Stripe ephemeral key for PaymentSheet.
func (s *Service) CreateEphemeralKey(customerID string) (*EphemeralKey, error) {
	params := url.Values{}
	params.Set("customer", customerID)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/ephemeral_keys", strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+s.secretKey)
	req.Header.Set("Stripe-Version", "2024-04-10")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ephemeral key request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe error %d: %s", resp.StatusCode, string(body))
	}

	var ek EphemeralKey
	if err := json.Unmarshal(body, &ek); err != nil {
		return nil, fmt.Errorf("decode ephemeral key: %w", err)
	}
	return &ek, nil
}

// --- PaymentIntent ---

// CreatePaymentIntent creates a Stripe PaymentIntent for mobile PaymentSheet.
func (s *Service) CreatePaymentIntent(amountCents int64, currency, customerID string, platformFeeCents int64, idempotencyKey string) (*PaymentIntent, error) {
	params := url.Values{}
	params.Set("amount", fmt.Sprintf("%d", amountCents))
	params.Set("currency", strings.ToLower(currency))
	params.Set("customer", customerID)
	params.Set("automatic_payment_methods[enabled]", "true")

	// TODO: When owner Stripe Connect accounts are implemented, add:
	// params.Set("application_fee_amount", fmt.Sprintf("%d", platformFeeCents))
	// params.Set("transfer_data[destination]", ownerStripeAccountID)
	// For now, platform collects full amount and records fee separately.

	params.Set("metadata[platform_fee_cents]", fmt.Sprintf("%d", platformFeeCents))

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/payment_intents", strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+s.secretKey)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe error %d: %s", resp.StatusCode, string(body))
	}

	var pi PaymentIntent
	if err := json.Unmarshal(body, &pi); err != nil {
		return nil, fmt.Errorf("decode payment intent: %w", err)
	}
	return &pi, nil
}

// --- Retrieve PaymentIntent ---

// RetrievePaymentIntent fetches the current state of a PaymentIntent from Stripe.
// Used as a fallback when webhooks are delayed or misconfigured.
func (s *Service) RetrievePaymentIntent(id string) (*PaymentIntent, error) {
	body, err := s.apiGet("/v1/payment_intents/" + id)
	if err != nil {
		return nil, fmt.Errorf("retrieve payment intent: %w", err)
	}
	var pi PaymentIntent
	if err := json.Unmarshal(body, &pi); err != nil {
		return nil, fmt.Errorf("decode payment intent: %w", err)
	}
	return &pi, nil
}

// --- Webhook signature verification ---

// VerifyWebhookSignature verifies a Stripe webhook signature using the official stripe-go library.
// Returns the parsed event as a generic map (matches existing handler interface).
func (s *Service) VerifyWebhookSignature(payload []byte, sigHeader string) (map[string]interface{}, error) {
	// Use stripe-go's official verification (handles tolerance, multiple v1 sigs, key rotation)
	event, err := webhook.ConstructEvent(payload, sigHeader, s.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("webhook signature verification: %w", err)
	}

	// Convert the verified event to a generic map for backward compatibility
	var result map[string]interface{}
	if err := json.Unmarshal(event.Data.Raw, &result); err != nil {
		return nil, fmt.Errorf("parse event data: %w", err)
	}

	// Build the map structure the handler expects: { "type": ..., "data": { "object": ... } }
	return map[string]interface{}{
		"type": string(event.Type),
		"data": map[string]interface{}{
			"object": result,
		},
	}, nil
}

// --- HTTP helpers ---

func (s *Service) apiGet(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", "https://api.stripe.com"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.secretKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (s *Service) apiPost(path string, params url.Values) ([]byte, error) {
	req, err := http.NewRequest("POST", "https://api.stripe.com"+path, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+s.secretKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe error %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
