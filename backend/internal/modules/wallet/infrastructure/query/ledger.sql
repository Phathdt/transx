-- name: InsertLedgerEntry :one
INSERT INTO ledger_entries (transfer_id, account_id, direction, amount, balance_after)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
