-- +goose Up
-- +goose StatementBegin
-- EXTERNAL transfers have no in-ledger destination account (funds leave the
-- system via a provider), so to_account_id must be optional. INTERNAL transfers
-- still require it, enforced in the application layer.
ALTER TABLE transfers
    ALTER COLUMN to_account_id DROP NOT NULL;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
-- Revert succeeds only if no EXTERNAL rows left a NULL to_account_id behind.
ALTER TABLE transfers
    ALTER COLUMN to_account_id SET NOT NULL;

-- +goose StatementEnd
