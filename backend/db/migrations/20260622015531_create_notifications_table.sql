-- +goose Up
-- +goose StatementBegin
-- Append-only audit of every notification dispatch. The notification service is
-- a downstream consumer of transfer.completed/failed; it deliberately keeps no
-- FK to transfers so it stays loosely coupled and can be deployed/scaled apart.
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    transfer_id uuid NOT NULL,
    event_type text NOT NULL,
    -- transfer.completed | transfer.failed
    channel text NOT NULL,
    -- EMAIL | PUSH
    recipient text NOT NULL,
    status text NOT NULL,
    -- SENT | FAILED
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_transfer_id ON notifications (transfer_id);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE notifications;

-- +goose StatementEnd
