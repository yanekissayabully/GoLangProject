-- Track when the user last viewed their Today actions (for unread dot).
ALTER TABLE users
    ADD COLUMN last_seen_actions_at TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01T00:00:00Z';
