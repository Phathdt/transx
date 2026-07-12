-- +goose Up
-- +goose StatementBegin
-- iris (CDC) drains outbox_events via the replication slot LSN, so the poll-era
-- status/published_at columns and their index are no longer read or written.
DROP INDEX IF EXISTS idx_outbox_events_status_created;

ALTER TABLE outbox_events
    DROP COLUMN status,
    DROP COLUMN published_at;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
ALTER TABLE outbox_events
    ADD COLUMN status text NOT NULL DEFAULT 'PENDING',
    ADD COLUMN published_at timestamptz;

CREATE INDEX idx_outbox_events_status_created ON outbox_events (status, created_at);

-- +goose StatementEnd
