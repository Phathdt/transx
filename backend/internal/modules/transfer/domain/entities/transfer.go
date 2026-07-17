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
	TransferStatusScheduled  TransferStatus = "SCHEDULED"
	TransferStatusReserved   TransferStatus = "RESERVED"
	TransferStatusProcessing TransferStatus = "PROCESSING"
	TransferStatusSubmitted  TransferStatus = "SUBMITTED"
	TransferStatusSucceeded  TransferStatus = "SUCCEEDED"
	TransferStatusFailed     TransferStatus = "FAILED"
	TransferStatusCancelled  TransferStatus = "CANCELLED"
	TransferStatusReversed   TransferStatus = "REVERSED"
	TransferStatusUnknown    TransferStatus = "UNKNOWN"
)

// Failure reasons recorded on a FAILED transfer. FailureCancelled marks a
// SCHEDULED transfer the owner cancelled before execute_at; it reuses the
// FAILED status + outbox event so the existing notification path (which only
// watches transfer.failed) surfaces it without a new event type.
const (
	FailureInsufficientFunds = "INSUFFICIENT_FUNDS"
	FailureAccountNotActive  = "ACCOUNT_NOT_ACTIVE"
	FailureDestNotActive     = "DEST_NOT_ACTIVE"
	FailureProviderRejected  = "PROVIDER_REJECTED"
	FailureFXRateUnavailable = "FX_RATE_UNAVAILABLE"
	FailureCancelled         = "CANCELLED"
)

// Transfer is a movement of funds. INTERNAL transfers move funds between two
// in-ledger accounts; EXTERNAL transfers send funds out through a provider and
// carry no in-ledger destination (ToAccountRef is empty / NULL).
//
// Accounts are referenced by their external text ref (ACC- + ULID), not the
// internal UUID. FromAccountRef always names an in-system account; ToAccountRef
// is an in-system account for INTERNAL transfers or a free-text beneficiary for
// EXTERNAL ones (no FK), and is empty when there is no destination.
type Transfer struct {
	ID                  uuid.UUID
	Reference           string
	FromAccountRef      string
	ToAccountRef        string
	ToAccountName       string
	TransactionAmount   decimal.Decimal
	TransactionCurrency string
	SourceAmount        decimal.NullDecimal
	SourceCurrency      string
	DestinationAmount   decimal.NullDecimal
	DestinationCurrency string
	SourceFXRate        decimal.NullDecimal
	DestinationFXRate   decimal.NullDecimal
	FeeAmount           decimal.Decimal
	FeeCurrency         string
	TransferType        string
	Provider            string
	ProviderReferenceID string
	Status              TransferStatus
	FailureReason       string
	Message             string
	UserID              uuid.UUID
	IdempotencyKey      string
	RequestHash         string
	// ExecuteAt is nil for an immediate transfer. When set, the transfer starts
	// in SCHEDULED and the Temporal workflow waits until this time before
	// entering the INTERNAL/EXTERNAL saga.
	ExecuteAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
