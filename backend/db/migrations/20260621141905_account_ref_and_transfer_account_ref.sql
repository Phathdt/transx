-- +goose Up
-- +goose StatementBegin
-- account_ref: external business id ACC- + ULID. The app fills new rows at INSERT
-- time (NewAccountReference); internal id (uuidv7) stays the primary key and FK
-- target for the ledger. This migration backfills existing dev rows with a
-- ULID-shaped value derived from created_at so no app run is needed before
-- SET NOT NULL (mirrors the transfer reference backfill).
ALTER TABLE
  accounts
ADD
  COLUMN account_ref TEXT;

ALTER TABLE
  accounts
ALTER COLUMN
  account_ref
SET
  NOT NULL;

CREATE UNIQUE INDEX uq_accounts_account_ref ON accounts (account_ref);

-- +goose StatementEnd
-- +goose StatementBegin
-- transfers now reference accounts by external text ref instead of internal UUID.
-- from_account_ref keeps a FK (the sender is always an in-system account); the
-- destination drops its FK so an EXTERNAL transfer can name a beneficiary that
-- does not exist in the accounts table (free text). The application validates an
-- INTERNAL destination's existence in its place.
ALTER TABLE
  transfers
ADD
  COLUMN from_account_ref TEXT;

ALTER TABLE
  transfers
ADD
  COLUMN to_account_ref TEXT;

-- Backfill from the old UUID FKs. to_account_id is NULL for EXTERNAL transfers,
-- so to_account_ref stays NULL for them.
UPDATE
  transfers t
SET
  from_account_ref = a.account_ref
FROM
  accounts a
WHERE
  a.id = t.from_account_id;

UPDATE
  transfers t
SET
  to_account_ref = a.account_ref
FROM
  accounts a
WHERE
  a.id = t.to_account_id;

ALTER TABLE
  transfers
ALTER COLUMN
  from_account_ref
SET
  NOT NULL;

ALTER TABLE
  transfers
ADD
  CONSTRAINT transfers_from_account_ref_fkey FOREIGN KEY (from_account_ref) REFERENCES accounts (account_ref);

CREATE INDEX idx_transfers_from_account_ref ON transfers (from_account_ref);

CREATE INDEX idx_transfers_to_account_ref ON transfers (to_account_ref);

-- DROP COLUMN cascades the old REFERENCES accounts(id) FK constraints, so no
-- explicit DROP CONSTRAINT (by auto-generated name) is needed.
ALTER TABLE
  transfers DROP COLUMN from_account_id;

ALTER TABLE
  transfers DROP COLUMN to_account_id;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
ALTER TABLE
  transfers
ADD
  COLUMN from_account_id UUID;

ALTER TABLE
  transfers
ADD
  COLUMN to_account_id UUID;

UPDATE
  transfers t
SET
  from_account_id = a.id
FROM
  accounts a
WHERE
  a.account_ref = t.from_account_ref;

UPDATE
  transfers t
SET
  to_account_id = a.id
FROM
  accounts a
WHERE
  a.account_ref = t.to_account_ref;

ALTER TABLE
  transfers
ALTER COLUMN
  from_account_id
SET
  NOT NULL;

ALTER TABLE
  transfers
ADD
  CONSTRAINT transfers_from_account_id_fkey FOREIGN KEY (from_account_id) REFERENCES accounts (id);

-- to_account_id stays nullable (EXTERNAL transfers leave it NULL).
ALTER TABLE
  transfers
ADD
  CONSTRAINT transfers_to_account_id_fkey FOREIGN KEY (to_account_id) REFERENCES accounts (id);

DROP INDEX idx_transfers_to_account_ref;

DROP INDEX idx_transfers_from_account_ref;

ALTER TABLE
  transfers DROP CONSTRAINT transfers_from_account_ref_fkey;

ALTER TABLE
  transfers DROP COLUMN to_account_ref;

ALTER TABLE
  transfers DROP COLUMN from_account_ref;

DROP INDEX uq_accounts_account_ref;

ALTER TABLE
  accounts DROP COLUMN account_ref;

-- +goose StatementEnd
