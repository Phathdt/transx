-- +goose Up
-- +goose StatementBegin
-- Scheduled transfers stage an intent now and execute later: execute_at is the
-- time the Temporal workflow wakes up and runs the existing INTERNAL/EXTERNAL
-- saga. NULL means immediate (today's behavior, unchanged).
ALTER TABLE transfers
    ADD COLUMN execute_at timestamptz NULL;

COMMENT ON COLUMN transfers.status IS 'PENDING | SCHEDULED | RESERVED | PROCESSING | SUBMITTED | SUCCEEDED | FAILED | CANCELLED | REVERSED | UNKNOWN';

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
COMMENT ON COLUMN transfers.status IS 'PENDING | RESERVED | PROCESSING | SUBMITTED | SUCCEEDED | FAILED | REVERSED | UNKNOWN';

ALTER TABLE transfers
    DROP COLUMN execute_at;

-- +goose StatementEnd
