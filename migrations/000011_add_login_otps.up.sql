-- Login OTPs: email-based passwordless login (separate from email_otps which are user-bound)
CREATE TABLE login_otps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL,
    code_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    consumed_at TIMESTAMP WITH TIME ZONE,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- For finding the latest unconsumed OTP for an email
CREATE INDEX idx_login_otps_email_active ON login_otps(email) WHERE consumed_at IS NULL;
-- For rate-limit queries (count recent OTPs per email in time window)
CREATE INDEX idx_login_otps_email_created ON login_otps(email, created_at);
