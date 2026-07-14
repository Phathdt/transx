package interfaces

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Sentinel errors surfaced by MoneyRepository. Callers (e.g. the Wallet gRPC
// handler) map these onto transport-specific error codes.
var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrAccountNotActive  = errors.New("account not active")
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrCurrencyMismatch means the caller-supplied currency does not match the
	// account's own currency. MoneyRepository does not convert currency; the
	// caller (e.g. an FX quote upstream of this call) must supply amounts
	// already denominated in each account's currency.
	ErrCurrencyMismatch = errors.New("currency mismatch")
)

// Operation names recorded in the wallet operation guard. A gRPC handler (or
// any other caller) supplies one of these as the idempotency operation.
const (
	OperationMove        = "MOVE"
	OperationHold        = "HOLD"
	OperationSettleHold  = "SETTLE_HOLD"
	OperationReleaseHold = "RELEASE_HOLD"
)

// MoveInput is one Move operation. SourceAmount/DestinationAmount are already
// quoted into each account's own currency by the caller (e.g. an FX lookup
// upstream of this call); MoneyRepository does not quote. FeeAmount is charged
// on the source account alongside SourceAmount; a zero FeeAmount charges no fee.
type MoveInput struct {
	FromAccountRef      string
	ToAccountRef        string
	SourceAmount        decimal.Decimal
	SourceCurrency      string
	DestinationAmount   decimal.Decimal
	DestinationCurrency string
	FeeAmount           decimal.Decimal
	FeeCurrency         string
}

// MoveResult is the post-operation available balance of both accounts.
type MoveResult struct {
	FromAvailableBalance decimal.Decimal
	ToAvailableBalance   decimal.Decimal
}

// HoldResult is the post-operation available and hold balance of one account.
type HoldResult struct {
	AvailableBalance decimal.Decimal
	HoldBalance      decimal.Decimal
}

// MoneyRepository performs idempotent money movements over accounts and
// holds. Every method is keyed by (transferID, operation): a call repeating a
// (transferID, operation) pair that already committed is a no-op that returns
// the account's current balance instead of reapplying the movement. Callers
// (e.g. a Temporal activity retry or a gRPC client redelivery) rely on this to
// make retries safe.
type MoneyRepository interface {
	// Move debits FromAccountRef by SourceAmount+FeeAmount and credits
	// ToAccountRef by DestinationAmount, writing the matching ledger entries,
	// all in one transaction.
	Move(ctx context.Context, transferID uuid.UUID, operation string, in MoveInput) (MoveResult, error)
	// Hold moves amount from available to hold on accountRef.
	Hold(
		ctx context.Context,
		transferID uuid.UUID,
		operation, accountRef string,
		amount decimal.Decimal,
		currency string,
	) (HoldResult, error)
	// SettleHold permanently drops a previously placed hold.
	SettleHold(
		ctx context.Context,
		transferID uuid.UUID,
		operation, accountRef string,
		amount decimal.Decimal,
		currency string,
	) (HoldResult, error)
	// ReleaseHold returns a previously placed hold back to available balance.
	ReleaseHold(
		ctx context.Context,
		transferID uuid.UUID,
		operation, accountRef string,
		amount decimal.Decimal,
		currency string,
	) (HoldResult, error)
}
