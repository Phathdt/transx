-- name: InsertNotification :one
INSERT INTO notifications (transfer_id, event_type, channel, recipient, status, error)
    VALUES ($1, $2, $3, $4, $5, $6)
RETURNING
    *;

-- name: GetTransferNotificationContext :one
-- Reloads the data needed to build a transfer notification by joining the
-- transfer to its sender account and that account's user. The transfer.* event
-- payload carries only {transferId}; everything else is read back here so the
-- event contract stays unchanged. The sender is always an in-system account
-- (transfers.from_account_ref FK + accounts.user_id NOT NULL + users.email
-- NOT NULL), so a matched row always has an email and user id.
SELECT
    transfers.reference,
    transfers.status,
    transfers.failure_reason,
    transfers.transaction_amount,
    transfers.transaction_currency,
    transfers.to_account_ref,
    users.id AS recipient_user_id,
    users.email AS recipient_email,
    users.name AS recipient_name
FROM
    transfers
    JOIN accounts ON accounts.account_ref = transfers.from_account_ref
    JOIN users ON users.id = accounts.user_id
WHERE
    transfers.id = $1;

