-- Add expires_at to lease_requests so owners see a deadline in the Today tab.
ALTER TABLE lease_requests
    ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours';

-- Back-fill existing rows: expires_at = created_at + 24h
UPDATE lease_requests SET expires_at = created_at + INTERVAL '24 hours';

-- Partial index for fast pending-action queries
CREATE INDEX idx_lease_requests_expires_at
    ON lease_requests (expires_at)
    WHERE status = 'requested';
