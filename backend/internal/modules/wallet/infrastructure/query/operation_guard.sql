-- name: WalletOperationGuardExists :one
-- Checked before a money movement, inside the same transaction as the movement
-- and the InsertWalletOperationGuard call below. A true result means this
-- exact (transfer_id, operation) already committed once, so the caller must
-- skip the movement and return the current account state instead.
SELECT
    EXISTS (
        SELECT
            1
        FROM
            wallet_operation_guards
        WHERE
            transfer_id = $1
            AND operation = $2);

-- name: InsertWalletOperationGuard :exec
-- Recorded after a money movement commits, inside the same transaction as the
-- movement. The unique index on (transfer_id, operation) backstops a
-- concurrent duplicate call: if two callers race past the exists-check, only
-- one insert succeeds and the tx of the loser must roll back rather than
-- double-apply the movement.
INSERT INTO wallet_operation_guards (transfer_id, operation)
    VALUES ($1, $2);

