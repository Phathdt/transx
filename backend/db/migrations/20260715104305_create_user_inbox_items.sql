-- +goose Up
-- +goose StatementBegin
-- Create user_inbox_items table for in-app inbox notifications.
-- This is separate from the notifications dispatch audit table; items here
-- are user-facing messages (not EMAIL/PUSH logs). Each row is an unread/read
-- message for one recipient on one terminal transfer event.
CREATE TABLE user_inbox_items (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type text NOT NULL, -- transfer.completed | transfer.failed
    title text NOT NULL,
    body text NOT NULL,
    transfer_id uuid REFERENCES transfers (id) ON DELETE SET NULL,
    transfer_ref text NULL, -- business reference (ITN-/ETN- + ULID)
    read_at timestamptz NULL, -- null = unread
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Partial unique: when transfer_id is present, only one row per user+type+transfer.
-- MVP transfer events always set transfer_id, so Kafka redelivery is idempotent.
CREATE UNIQUE INDEX idx_user_inbox_items_unique ON user_inbox_items (user_id, type, transfer_id)
WHERE
    transfer_id IS NOT NULL;

-- Ordering for inbox listing (newest first).
CREATE INDEX idx_user_inbox_items_user_created ON user_inbox_items (user_id, created_at DESC);

-- Fast unread count: only rows where read_at IS NULL.
CREATE INDEX idx_user_inbox_items_unread ON user_inbox_items (user_id)
WHERE
    read_at IS NULL;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_inbox_items;

-- +goose StatementEnd
