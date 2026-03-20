-- Migration 000006: Add chat/messaging system
-- Creates: chats, chat_participants, messages, requests, attachments

-- Enum types
CREATE TYPE message_type AS ENUM ('text', 'system');
CREATE TYPE request_type AS ENUM ('manual_payment', 'delayed_payment', 'mechanic_service', 'additional_fee', 'generic');
CREATE TYPE request_status AS ENUM ('pending', 'accepted', 'declined', 'expired', 'cancelled');
CREATE TYPE attachment_kind AS ENUM ('image', 'document', 'video');

-- Chats table: one chat per (car, driver, owner) triple
CREATE TABLE chats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    car_id UUID NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    driver_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_message_at TIMESTAMPTZ,
    last_request_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_chats_car_driver_owner UNIQUE (car_id, driver_id, owner_id)
);

CREATE INDEX idx_chats_driver_id ON chats(driver_id);
CREATE INDEX idx_chats_owner_id ON chats(owner_id);

CREATE TRIGGER trigger_chats_updated_at
    BEFORE UPDATE ON chats
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Chat participants: per-user read state and settings
CREATE TABLE chat_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_at TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    auto_translate BOOLEAN NOT NULL DEFAULT false,
    notifications_muted BOOLEAN NOT NULL DEFAULT false,
    is_archived BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_chat_participants UNIQUE (chat_id, user_id)
);

CREATE INDEX idx_chat_participants_user_id ON chat_participants(user_id);

CREATE TRIGGER trigger_chat_participants_updated_at
    BEFORE UPDATE ON chat_participants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Requests table (created before messages so messages can FK to it)
CREATE TABLE requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    type request_type NOT NULL,
    status request_status NOT NULL DEFAULT 'pending',
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount DECIMAL(12, 2),
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    payload_json JSONB NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_requests_chat_status ON requests(chat_id, status);
CREATE INDEX idx_requests_expires ON requests(expires_at) WHERE status = 'pending';

CREATE TRIGGER trigger_requests_updated_at
    BEFORE UPDATE ON requests
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Messages table
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type message_type NOT NULL DEFAULT 'text',
    body TEXT NOT NULL,
    client_message_id UUID,
    request_id UUID REFERENCES requests(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_chat_created ON messages(chat_id, created_at);
CREATE UNIQUE INDEX idx_messages_client_id ON messages(client_message_id) WHERE client_message_id IS NOT NULL;

-- Attachments table
CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    request_id UUID REFERENCES requests(id) ON DELETE SET NULL,
    uploader_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind attachment_kind NOT NULL,
    filename VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    file_size INT NOT NULL DEFAULT 0,
    file_path VARCHAR(500) NOT NULL,
    file_url VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_attachments_chat_kind ON attachments(chat_id, kind, created_at);
CREATE INDEX idx_attachments_message ON attachments(message_id) WHERE message_id IS NOT NULL;
CREATE INDEX idx_attachments_request ON attachments(request_id) WHERE request_id IS NOT NULL;
