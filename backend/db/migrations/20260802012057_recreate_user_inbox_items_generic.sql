-- +goose Up
-- +goose StatementBegin
-- Inbox items are generic over notification source so a non-transfer source
-- (system, report) can be inserted without a schema change. The unique index is
-- full, not partial: a partial index on a nullable transfer_id silently skipped
-- rows with no transfer, making ON CONFLICT a no-match and letting producer
-- retries duplicate. No FK to transfers keeps this table deployable apart from
-- the transfer module, matching the notifications table.
DROP TABLE IF EXISTS user_inbox_items;

CREATE TABLE user_inbox_items (
  id uuid PRIMARY KEY DEFAULT uuidv7 (),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  type text NOT NULL,
  -- transfer.completed | transfer.failed | system.* | report.*
  title text NOT NULL,
  body text NOT NULL,
  source_type text NOT NULL,
  -- transfer | system | report
  source_id text NOT NULL,
  -- transfer: transfers.id as text; system: producer-chosen stable key
  source_ref text NOT NULL DEFAULT '',
  -- display + deep-link reference (ITN-/ETN-…); empty when the source has none
  read_at timestamptz NULL,
  -- null = unread
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_user_inbox_items_unique ON user_inbox_items (user_id, type, source_type, source_id);

-- Ordering for inbox listing (newest first).
CREATE INDEX idx_user_inbox_items_user_created ON user_inbox_items (user_id, created_at DESC);

-- Fast unread count: only rows where read_at IS NULL.
CREATE INDEX idx_user_inbox_items_unread ON user_inbox_items (user_id)
WHERE
  read_at IS NULL;

-- Existing audit rows are keyed only by transfer_id, which this migration drops,
-- so they cannot be carried into the generic shape and are discarded. Truncating
-- also lets the new NOT NULL columns be added without a placeholder default.
TRUNCATE notifications;

ALTER TABLE
  notifications DROP COLUMN transfer_id,
ADD
  COLUMN source_type text NOT NULL,
ADD
  COLUMN source_id text NOT NULL;

DROP INDEX IF EXISTS idx_notifications_transfer_id;

CREATE INDEX idx_notifications_source ON notifications (source_type, source_id);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
-- Known limitation: this drops user_inbox_items without restoring the
-- previous transfer_id-keyed shape, so a binary from before this migration
-- will fail its inbox requests (relation does not exist) if redeployed after
-- a down migration. Accepted for this greenfield project: rollback here means
-- restoring a DB snapshot alongside the previous binary, not running the app
-- against a goose-downed schema.
DROP TABLE IF EXISTS user_inbox_items;

TRUNCATE notifications;

ALTER TABLE
  notifications DROP COLUMN source_type,
  DROP COLUMN source_id,
ADD
  COLUMN transfer_id uuid NOT NULL;

DROP INDEX IF EXISTS idx_notifications_source;

CREATE INDEX idx_notifications_transfer_id ON notifications (transfer_id);

-- +goose StatementEnd
