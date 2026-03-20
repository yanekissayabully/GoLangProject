-- Drop password reset tokens table
DROP TABLE IF EXISTS password_reset_tokens;

-- Drop documents table
DROP TABLE IF EXISTS documents;

-- Remove columns from users
ALTER TABLE users
    DROP COLUMN IF EXISTS onboarding_status,
    DROP COLUMN IF EXISTS profile_photo_url;

-- Drop onboarding status enum
DROP TYPE IF EXISTS onboarding_status;
