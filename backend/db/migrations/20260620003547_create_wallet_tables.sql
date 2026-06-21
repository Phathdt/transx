-- +goose Up
-- +goose StatementBegin
CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    user_id uuid NOT NULL,
    name text NOT NULL DEFAULT '',
    -- ISO-4217 code; the allow-list is enforced in the application layer.
    currency text NOT NULL,
    available_balance numeric(20, 4) NOT NULL DEFAULT 0 CHECK (available_balance >= 0),
    hold_balance numeric(20, 4) NOT NULL DEFAULT 0 CHECK (hold_balance >= 0),
    -- ACTIVE | FROZEN | CLOSED
    status text NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_accounts_user_id ON accounts (user_id);

-- Lets the seed command upsert by (user_id, name) and prevents duplicate wallet
-- names per user.
CREATE UNIQUE INDEX uq_accounts_user_name ON accounts (user_id, name);

-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE transfers (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    from_account_id uuid NOT NULL REFERENCES accounts (id),
    to_account_id uuid NOT NULL REFERENCES accounts (id),
    amount numeric(20, 4) NOT NULL CHECK (amount > 0),
    currency text NOT NULL,
    transfer_type text NOT NULL DEFAULT 'INTERNAL',
    provider text NOT NULL DEFAULT '',
    provider_reference_id text NOT NULL DEFAULT '',
    -- PENDING | RESERVED | PROCESSING | SUBMITTED | SUCCEEDED | FAILED | REVERSED | UNKNOWN
    status text NOT NULL DEFAULT 'PENDING',
    failure_reason text NOT NULL DEFAULT '',
    -- Owner (X-User-Id). Required to scope idempotency keys per caller.
    user_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    -- Canonical hash of the request body. Reusing a key with a different body is
    -- rejected by comparing this hash instead of returning the prior transfer.
    request_hash text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_transfers_user_key ON transfers (user_id, idempotency_key);

CREATE INDEX idx_transfers_status ON transfers (status);

-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE ledger_entries (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    transfer_id uuid NOT NULL REFERENCES transfers (id),
    account_id uuid NOT NULL REFERENCES accounts (id),
    -- DEBIT | CREDIT | HOLD | RELEASE
    direction text NOT NULL,
    amount numeric(20, 4) NOT NULL CHECK (amount > 0),
    balance_after numeric(20, 4) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_entries_transfer_id ON ledger_entries (transfer_id);

CREATE INDEX idx_ledger_entries_account_id ON ledger_entries (account_id);

-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE outbox_events (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    -- transfer.requested | transfer.completed | transfer.failed
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    -- PENDING | PUBLISHED
    status text NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);

-- Drives the FIFO outbox poll (status='PENDING' ORDER BY created_at).
CREATE INDEX idx_outbox_events_status_created ON outbox_events (status, created_at);

-- +goose StatementEnd
-- +goose StatementBegin
-- inbox_events deduplicates consumed messages, mirroring outbox_events. A
-- message processed successfully is recorded here so a later redelivery is a
-- no-op. Shared by every wallet consumer (currently the transfer processor).
CREATE TABLE inbox_events (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    consumer_group text NOT NULL,
    message_key text NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_inbox_group_key ON inbox_events (consumer_group, message_key);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE inbox_events;

-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE ledger_entries;

-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE outbox_events;

-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE transfers;

-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE accounts;

-- +goose StatementEnd
