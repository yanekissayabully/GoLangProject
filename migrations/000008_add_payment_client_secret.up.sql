-- Migration 000008: Add client_secret column to payments table
-- Stores Stripe PaymentIntent client_secret so it can be returned on retry.
-- The client_secret is designed to be sent to frontend clients (not a true secret).

ALTER TABLE payments ADD COLUMN payment_intent_client_secret TEXT;
