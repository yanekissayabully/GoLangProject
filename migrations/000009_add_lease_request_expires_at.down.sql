DROP INDEX IF EXISTS idx_lease_requests_expires_at;
ALTER TABLE lease_requests DROP COLUMN IF EXISTS expires_at;
