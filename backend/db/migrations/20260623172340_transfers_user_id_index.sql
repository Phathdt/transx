-- +goose Up
-- +goose StatementBegin
CREATE INDEX idx_transfers_user_created ON transfers (user_id, created_at DESC);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_transfers_user_created;

-- +goose StatementEnd
