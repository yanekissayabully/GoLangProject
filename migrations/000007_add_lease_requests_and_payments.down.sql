-- Rollback migration 000007

DROP TRIGGER IF EXISTS set_payments_updated_at ON payments;
DROP TRIGGER IF EXISTS set_lease_requests_updated_at ON lease_requests;

DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS lease_requests;

DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS lease_request_status;
