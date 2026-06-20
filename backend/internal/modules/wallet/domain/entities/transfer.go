package entities

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TransferStatus enumerates the lifecycle of a transfer. Internal transfers move
// PENDING → PROCESSING → SUCCEEDED (or → FAILED) within a single processor tx.
type TransferStatus string

const (
	TransferStatusPending    TransferStatus = "PENDING"
	TransferStatusReserved   TransferStatus = "RESERVED"
	TransferStatusProcessing TransferStatus = "PROCESSING"
	TransferStatusSubmitted  TransferStatus = "SUBMITTED"
	TransferStatusSucceeded  TransferStatus = "SUCCEEDED"
	TransferStatusFailed     TransferStatus = "FAILED"
	TransferStatusReversed   TransferStatus = "REVERSED"
	TransferStatusUnknown    TransferStatus = "UNKNOWN"
)

// Failure reasons recorded on a FAILED transfer.
const (
	FailureInsufficientFunds = "INSUFFICIENT_FUNDS"
	FailureAccountNotActive  = "ACCOUNT_NOT_ACTIVE"
	FailureDestNotActive     = "DEST_NOT_ACTIVE"
	FailureProviderRejected  = "PROVIDER_REJECTED"
)

// Transfer is a movement of funds. INTERNAL transfers move funds between two
// in-ledger accounts; EXTERNAL transfers send funds out through a provider and
// carry no in-ledger destination (ToAccountID is uuid.Nil / NULL).
type Transfer struct {
	ID                  uuid.UUID
	Reference           string
	FromAccountID       uuid.UUID
	ToAccountID         uuid.UUID
	Amount              decimal.Decimal
	Currency            string
	TransferType        string
	Provider            string
	ProviderReferenceID string
	Status              TransferStatus
	FailureReason       string
	UserID              uuid.UUID
	IdempotencyKey      string
	RequestHash         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
