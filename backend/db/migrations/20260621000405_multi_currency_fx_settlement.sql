-- +goose Up
-- +goose StatementBegin
-- Rename the client-requested transfer money into explicit transaction intent.
-- Settlement fields stay empty until the consumer computes the account-base postings.
ALTER TABLE transfers RENAME COLUMN amount TO transaction_amount;
ALTER TABLE transfers RENAME COLUMN currency TO transaction_currency;

ALTER TABLE transfers
    ADD COLUMN source_amount NUMERIC(20, 4) CHECK (source_amount IS NULL OR source_amount > 0),
    ADD COLUMN source_currency TEXT NOT NULL DEFAULT '',
    ADD COLUMN destination_amount NUMERIC(20, 4) CHECK (destination_amount IS NULL OR destination_amount > 0),
    ADD COLUMN destination_currency TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_fx_rate NUMERIC(20, 12) CHECK (source_fx_rate IS NULL OR source_fx_rate > 0),
    ADD COLUMN destination_fx_rate NUMERIC(20, 12) CHECK (destination_fx_rate IS NULL OR destination_fx_rate > 0),
    ADD COLUMN fee_amount NUMERIC(20, 4) NOT NULL DEFAULT 0 CHECK (fee_amount >= 0),
    ADD COLUMN fee_currency TEXT NOT NULL DEFAULT '';

ALTER TABLE ledger_entries ADD COLUMN currency TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE ledger_entries DROP COLUMN currency;

ALTER TABLE transfers
    DROP COLUMN fee_currency,
    DROP COLUMN fee_amount,
    DROP COLUMN destination_fx_rate,
    DROP COLUMN source_fx_rate,
    DROP COLUMN destination_currency,
    DROP COLUMN destination_amount,
    DROP COLUMN source_currency,
    DROP COLUMN source_amount;

ALTER TABLE transfers RENAME COLUMN transaction_currency TO currency;
ALTER TABLE transfers RENAME COLUMN transaction_amount TO amount;
-- +goose StatementEnd
