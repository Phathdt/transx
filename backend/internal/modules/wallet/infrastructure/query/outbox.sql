-- name: InsertOutboxEvent :one
INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPendingOutbox :many
-- FIFO poll. No row lock: a single publisher owns this table (see plan RT#7), so
-- ordering is preserved by created_at and dedup by MarkOutboxPublished's guard.
SELECT *
FROM outbox_events
WHERE status = 'PENDING'
ORDER BY created_at
LIMIT $1;

-- name: MarkOutboxPublished :execrows
-- The status='PENDING' guard makes a re-publish of an already-published row a
-- no-op (0 rows affected), so an at-least-once publisher cannot double-mark.
UPDATE outbox_events
SET status = 'PUBLISHED', published_at = now()
WHERE id = $1 AND status = 'PENDING';
