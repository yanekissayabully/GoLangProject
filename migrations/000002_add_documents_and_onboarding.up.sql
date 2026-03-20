-- Add onboarding status enum
CREATE TYPE onboarding_status AS ENUM (
    'created',
    'role_selected',
    'photo_uploaded',
    'documents_uploaded',
    'complete'
);

-- Add new columns to users table
ALTER TABLE users
    ADD COLUMN onboarding_status onboarding_status NOT NULL DEFAULT 'created',
    ADD COLUMN profile_photo_url VARCHAR(500);

-- Update existing users to complete status (they already have role)
UPDATE users SET onboarding_status = 'complete' WHERE role IS NOT NULL;

-- Documents table for driver uploads
CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL, -- 'drivers_license' or 'registration'
    file_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_size BIGINT NOT NULL,
    mime_type VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'uploaded', -- 'uploaded', 'verified', 'rejected'
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for user document lookups
CREATE INDEX idx_documents_user_id ON documents(user_id);
CREATE UNIQUE INDEX idx_documents_user_type ON documents(user_id, type);

-- Trigger for documents updated_at
CREATE TRIGGER update_documents_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Password reset tokens table (replaces OTP for password reset)
CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for token lookups
CREATE INDEX idx_password_reset_tokens_user ON password_reset_tokens(user_id) WHERE used_at IS NULL;
CREATE INDEX idx_password_reset_tokens_hash ON password_reset_tokens(token_hash) WHERE used_at IS NULL;
