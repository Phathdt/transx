-- +goose Up
-- +goose StatementBegin
-- Dedup table for the Wallet gRPC money operations (Move/Hold/SettleHold/
-- ReleaseHold). Each RPC is keyed by (transfer_id, operation): a repeated call
-- with the same pair is a no-op after the first successful commit, so a caller
-- retry or redelivery cannot double-post money. Written in the same
-- transaction as the balance/ledger change it guards.
CREATE TABLE wallet_operation_guards (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    transfer_id uuid NOT NULL,
    -- MOVE | HOLD | SETTLE_HOLD | RELEASE_HOLD
    operation text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_wallet_operation_guards_transfer_operation ON wallet_operation_guards (transfer_id, operation);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE wallet_operation_guards;

-- +goose StatementEnd
