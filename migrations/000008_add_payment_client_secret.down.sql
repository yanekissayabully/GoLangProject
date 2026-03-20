-- Rollback migration 000008
ALTER TABLE payments DROP COLUMN IF EXISTS payment_intent_client_secret;
