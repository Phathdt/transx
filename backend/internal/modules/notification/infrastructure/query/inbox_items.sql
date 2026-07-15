-- name: InsertInboxItem :one
-- Inserts a user inbox item. ON CONFLICT updates title (no-op when equal) so
-- Kafka redelivery is safe and RETURNING still yields the existing row.
INSERT INTO user_inbox_items (user_id, type, title, body, transfer_id, transfer_ref)
    VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, type, transfer_id)
WHERE
    transfer_id IS NOT NULL
        DO UPDATE SET
            title = EXCLUDED.title
        RETURNING
            *;

-- name: GetInboxItemByUserAndID :one
SELECT
    id,
    user_id,
    type,
    title,
    body,
    transfer_id,
    transfer_ref,
    read_at,
    created_at
FROM
    user_inbox_items
WHERE
    id = $1
    AND user_id = $2;

-- name: ListInboxByUser :many
SELECT
    id,
    user_id,
    type,
    title,
    body,
    transfer_id,
    transfer_ref,
    read_at,
    created_at
FROM
    user_inbox_items
WHERE
    user_id = $1
ORDER BY
    created_at DESC,
    id DESC
LIMIT sqlc.arg ('lim') OFFSET sqlc.arg ('off');

-- name: CountInboxByUser :one
SELECT
    count(*)
FROM
    user_inbox_items
WHERE
    user_id = $1;

-- name: CountUnreadByUser :one
SELECT
    count(*)
FROM
    user_inbox_items
WHERE
    user_id = $1
    AND read_at IS NULL;

-- name: MarkInboxRead :one
-- Marks the item read if still unread; preserves the original read_at when the
-- client re-opens an already-read item. Ownership is enforced by user_id.
UPDATE
    user_inbox_items
SET
    read_at = COALESCE(read_at, now())
WHERE
    id = $1
    AND user_id = $2
RETURNING
    *;

-- name: MarkAllInboxRead :execrows
UPDATE
    user_inbox_items
SET
    read_at = now()
WHERE
    user_id = $1
    AND read_at IS NULL;

