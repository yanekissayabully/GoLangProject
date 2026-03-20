-- Rollback migration 000006: Remove chat/messaging system

DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS requests;
DROP TABLE IF EXISTS chat_participants;
DROP TABLE IF EXISTS chats;

DROP TYPE IF EXISTS attachment_kind;
DROP TYPE IF EXISTS request_status;
DROP TYPE IF EXISTS request_type;
DROP TYPE IF EXISTS message_type;
