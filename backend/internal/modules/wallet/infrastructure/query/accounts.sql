-- name: CreateAccount :one
INSERT INTO accounts (user_id, name, currency, available_balance, hold_balance, status, account_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetAccountByID :one
SELECT *
FROM accounts
WHERE id = $1;

-- name: GetAccountByRef :one
SELECT *
FROM accounts
WHERE account_ref = $1;

-- name: GetAccountByRefForUser :one
SELECT *
FROM accounts
WHERE account_ref = $1 AND user_id = $2;

-- name: LockAccountsByRefs :many
-- Locks the given accounts in a deterministic order (ORDER BY account_ref) so
-- two crossing transfers (A->B and B->A) cannot deadlock on lock acquisition.
-- Internal balance/ledger work still keys off the UUID id carried on each row.
SELECT *
FROM accounts
WHERE account_ref = ANY(@refs::text [])
ORDER BY account_ref
FOR UPDATE;

-- name: UpdateAccountStatus :exec
UPDATE accounts
SET status = $1, updated_at = now()
WHERE id = $2;

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

-- name: ReserveHoldIfSufficient :one
-- Reserve: move funds from available into hold for an external transfer. Only
-- succeeds for an ACTIVE account with enough available funds; no row updated
-- means insufficient funds (status validated as ACTIVE by the caller's lock).
UPDATE accounts
SET available_balance = available_balance - @amount,
    hold_balance      = hold_balance + @amount,
    updated_at        = now()
WHERE id = @id
  AND status = 'ACTIVE'
  AND available_balance >= @amount
RETURNING available_balance, hold_balance;

-- name: DebitHold :one
-- Settle success: drop the held amount permanently (funds left the system). The
-- account is already locked and the hold was placed in the reserve step, so no
-- conditional guard is needed; the CHECK (hold_balance >= 0) backstops underflow.
UPDATE accounts
SET hold_balance = hold_balance - @amount,
    updated_at   = now()
WHERE id = @id
RETURNING available_balance, hold_balance;

-- name: ReleaseHold :one
-- Settle failure: return the held amount to available balance.
UPDATE accounts
SET hold_balance      = hold_balance - @amount,
    available_balance = available_balance + @amount,
    updated_at        = now()
WHERE id = @id
RETURNING available_balance, hold_balance;
