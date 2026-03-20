-- Migration 000007: Add lease requests and payments tables
-- Supports the lease request + Stripe payment flow

-- Lease request status enum
CREATE TYPE lease_request_status AS ENUM (
    'requested',
    'accepted',
    'declined',
    'cancelled',
    'payment_pending',
    'paid',
    'expired'
);

-- Payment status enum (mirrors Stripe PaymentIntent statuses)
CREATE TYPE payment_status AS ENUM (
    'requires_payment_method',
    'requires_confirmation',
    'processing',
    'succeeded',
    'canceled',
    'failed'
);

-- Lease requests table
CREATE TABLE lease_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    listing_id      UUID NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    driver_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          lease_request_status NOT NULL DEFAULT 'requested',
    weekly_price    DECIMAL(12,2) NOT NULL,
    currency        VARCHAR(3) NOT NULL DEFAULT 'USD',
    weeks           INT NOT NULL DEFAULT 1,
    message         TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_lease_requests_chat_id ON lease_requests(chat_id);
CREATE INDEX idx_lease_requests_listing_id ON lease_requests(listing_id);
CREATE INDEX idx_lease_requests_driver_id ON lease_requests(driver_id);
CREATE INDEX idx_lease_requests_owner_id ON lease_requests(owner_id);
CREATE INDEX idx_lease_requests_status ON lease_requests(status);

-- Prevent duplicate active lease requests per driver+listing
CREATE UNIQUE INDEX idx_lease_requests_active_per_driver_listing
    ON lease_requests(driver_id, listing_id)
    WHERE status IN ('requested', 'accepted', 'payment_pending');

-- Payments table (Stripe)
CREATE TABLE payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lease_request_id    UUID NOT NULL REFERENCES lease_requests(id) ON DELETE CASCADE,
    provider            VARCHAR(20) NOT NULL DEFAULT 'stripe',
    stripe_customer_id  VARCHAR(255),
    payment_intent_id   VARCHAR(255),
    amount              BIGINT NOT NULL,          -- in smallest currency unit (cents)
    currency            VARCHAR(3) NOT NULL DEFAULT 'USD',
    platform_fee_amount BIGINT NOT NULL DEFAULT 0, -- platform commission in cents
    status              payment_status NOT NULL DEFAULT 'requires_payment_method',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One payment per lease request
CREATE UNIQUE INDEX idx_payments_lease_request_id ON payments(lease_request_id);
-- Lookup by Stripe PaymentIntent ID (for webhooks)
CREATE UNIQUE INDEX idx_payments_payment_intent_id ON payments(payment_intent_id) WHERE payment_intent_id IS NOT NULL;

-- Auto-update updated_at triggers
CREATE TRIGGER set_lease_requests_updated_at
    BEFORE UPDATE ON lease_requests
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER set_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
