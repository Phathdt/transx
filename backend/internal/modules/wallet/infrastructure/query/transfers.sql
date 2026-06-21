-- name: CreateTransfer :one
INSERT INTO transfers (
    from_account_id, to_account_id, transaction_amount, transaction_currency,
    transfer_type, provider, status, user_id, idempotency_key, request_hash,
    reference, fee_amount, fee_currency
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: SetTransferSettlementSnapshot :exec
UPDATE transfers
SET source_amount = @source_amount,
    source_currency = @source_currency,
    destination_amount = @destination_amount,
    destination_currency = @destination_currency,
    source_fx_rate = @source_fx_rate,
    destination_fx_rate = @destination_fx_rate,
    updated_at = now()
WHERE id = @id;

-- name: GetTransferByID :one
SELECT *
FROM transfers
WHERE id = $1;

-- name: GetTransferByReferenceForUser :one
SELECT *
FROM transfers
WHERE reference = $1 AND user_id = $2;

-- name: GetTransferByUserAndKey :one
SELECT *
FROM transfers
WHERE user_id = $1 AND idempotency_key = $2;

-- name: LockTransferByID :one
-- Serializes concurrent processing of the same transfer; paired with the
-- status='PENDING' guard to prevent double-credit on redelivery.
SELECT *
FROM transfers
WHERE id = $1
FOR UPDATE;

-- name: UpdateTransferStatus :exec
UPDATE transfers
SET status = @status, updated_at = now()
WHERE id = @id;

-- name: FailTransfer :exec
UPDATE transfers
SET status = 'FAILED', failure_reason = @failure_reason, updated_at = now()
WHERE id = @id;

-- name: SetProviderReference :exec
-- Stores the reference id returned by the provider on a successful submit.
UPDATE transfers
SET provider_reference_id = @provider_reference_id, updated_at = now()
WHERE id = @id;
