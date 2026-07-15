-- name: InsertNotification :one
INSERT INTO notifications (transfer_id, event_type, channel, recipient, status, error)
    VALUES ($1, $2, $3, $4, $5, $6)
RETURNING
    *;

-- name: GetTransferNotificationContext :one
-- Reloads the data needed to build a transfer notification / inbox item by
-- joining the transfer to its sender account+user and optionally the
-- destination account+user. The transfer.* event payload carries only
-- {transferId}; everything else is read back here so the event contract stays
-- unchanged. The sender is always an in-system account (from_account_ref FK +
-- accounts.user_id NOT NULL + users.email NOT NULL), so a matched row always
-- has a sender email and user id. Destination is LEFT JOINed so EXTERNAL
-- free-text refs leave to_user_id NULL.
SELECT
    transfers.reference,
    transfers.status,
    transfers.failure_reason,
    transfers.transaction_amount,
    transfers.transaction_currency,
    transfers.to_account_ref,
    transfers.transfer_type,
    from_users.id AS recipient_user_id,
    from_users.email AS recipient_email,
    from_users.name AS recipient_name,
    to_users.id AS to_user_id
FROM
    transfers
    JOIN accounts AS from_accounts ON from_accounts.account_ref = transfers.from_account_ref
    JOIN users AS from_users ON from_users.id = from_accounts.user_id
    LEFT JOIN accounts AS to_accounts ON to_accounts.account_ref = transfers.to_account_ref
    LEFT JOIN users AS to_users ON to_users.id = to_accounts.user_id
WHERE
    transfers.id = $1;

