-- +goose Up
-- +goose StatementBegin
-- to_account_name: holder-name snapshot of the destination account, captured at
-- create time so it stays stable if the account is later renamed. NULL for
-- EXTERNAL transfers (free-text beneficiary, no in-system holder) and for rows
-- created before this migration.
-- message: user-supplied transfer note, stored as-is.
ALTER TABLE transfers
    ADD COLUMN to_account_name TEXT,
    ADD COLUMN message TEXT;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
ALTER TABLE transfers
    DROP COLUMN to_account_name,
    DROP COLUMN message;

-- +goose StatementEnd
