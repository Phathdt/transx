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
)

// Transfer is a movement of funds between two accounts. For this scope only
// INTERNAL transfers are processed end-to-end.
type Transfer struct {
	ID                  uuid.UUID
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
