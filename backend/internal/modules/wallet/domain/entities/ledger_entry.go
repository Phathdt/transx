package entities

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LedgerDirection is the side of a ledger entry. A transfer produces a DEBIT on
// the source account and a CREDIT on the destination.
type LedgerDirection string

const (
	LedgerDebit   LedgerDirection = "DEBIT"
	LedgerCredit  LedgerDirection = "CREDIT"
	LedgerHold    LedgerDirection = "HOLD"
	LedgerRelease LedgerDirection = "RELEASE"
)

// LedgerEntry is an append-only record of a balance change. BalanceAfter is the
// account's available balance immediately after this entry, for audit.
type LedgerEntry struct {
	ID           uuid.UUID
	TransferID   uuid.UUID
	AccountID    uuid.UUID
	Direction    LedgerDirection
	Amount       decimal.Decimal
	BalanceAfter decimal.Decimal
	CreatedAt    time.Time
}
