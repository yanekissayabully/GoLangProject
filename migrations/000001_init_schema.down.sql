-- Drop triggers
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables
DROP TABLE IF EXISTS otp_rate_limits;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS email_otps;
DROP TABLE IF EXISTS users;

-- Drop enum types
DROP TYPE IF EXISTS otp_purpose;
DROP TYPE IF EXISTS user_role;

-- Drop extension
DROP EXTENSION IF EXISTS "uuid-ossp";
