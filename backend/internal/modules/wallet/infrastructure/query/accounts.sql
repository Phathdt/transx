-- name: CreateAccount :one
INSERT INTO accounts (user_id, name, currency, available_balance, hold_balance, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAccountByID :one
SELECT *
FROM accounts
WHERE id = $1;

-- name: GetAccountByIDForUser :one
SELECT *
FROM accounts
WHERE id = $1 AND user_id = $2;

-- name: LockAccountsByIDs :many
-- Locks the given accounts in a deterministic order (ORDER BY id) so two
-- crossing transfers (A->B and B->A) cannot deadlock on lock acquisition.
SELECT *
FROM accounts
WHERE id = ANY(@ids::uuid [])
ORDER BY id
FOR UPDATE;

-- name: DebitAvailableIfSufficient :one
-- Conditional debit: only succeeds for an ACTIVE account with enough funds. The
-- caller distinguishes "no row updated" causes by re-reading the account status.
UPDATE accounts
SET available_balance = available_balance - @amount,
    updated_at        = now()
WHERE id = @id
  AND status = 'ACTIVE'
  AND available_balance >= @amount
RETURNING available_balance;

-- name: CreditAvailable :one
-- Credit only lands on an ACTIVE account; a non-ACTIVE destination yields no row.
UPDATE accounts
SET available_balance = available_balance + @amount,
    updated_at        = now()
WHERE id = @id
  AND status = 'ACTIVE'
RETURNING available_balance;
