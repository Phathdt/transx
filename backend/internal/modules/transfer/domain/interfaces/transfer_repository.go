package interfaces

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"transx/internal/modules/transfer/domain/entities"
)

// TransferRepository persists and loads transfers, including the idempotency
// lookup and the transactional internal-transfer execution.
type TransferRepository interface {
	Create(ctx context.Context, t *entities.Transfer) (*entities.Transfer, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Transfer, error)
	// GetByReferenceForUser returns the transfer by its business reference
	// (ETN-/ITN- + ULID), scoped to the owner. Returns (nil, nil) when none.
	GetByReferenceForUser(ctx context.Context, reference string, userID uuid.UUID) (*entities.Transfer, error)
	// FindByUserAndKey looks up a transfer by its idempotency key, scoped to the
	// owner. Returns (nil, nil) when none exists.
	FindByUserAndKey(ctx context.Context, userID uuid.UUID, key string) (*entities.Transfer, error)
	// ExecuteInternalTransfer runs the debit/credit/ledger/status/outbox steps
	// for one transfer atomically in a single transaction. It is idempotent: a
	// transfer not in PENDING is skipped.
	ExecuteInternalTransfer(ctx context.Context, transferID uuid.UUID, fx FXService) error
	// ReserveExternalTransfer moves the amount from available to hold and stages
	// the provider-request outbox event in one transaction (PENDING → RESERVED).
	// Idempotent: a transfer not in PENDING is skipped.
	ReserveExternalTransfer(ctx context.Context, transferID uuid.UUID) error
	// SettleExternalTransfer applies the provider outcome in one transaction
	// (RESERVED → SUCCEEDED on success, → FAILED with hold released on failure).
	// Idempotent: a transfer not in RESERVED is skipped.
	SettleExternalTransfer(ctx context.Context, transferID uuid.UUID, result entities.ProviderResult) error
	// MarkTerminal advances the transfer status and outbox only (no wallet
	// mutation). On success the transfer is set SUCCEEDED with a
	// transfer.completed outbox event; on failure it is set FAILED with the
	// given reason and a transfer.failed outbox event. It is idempotent: a
	// transfer already in a terminal status (SUCCEEDED, FAILED) is a no-op.
	// PENDING, PROCESSING and RESERVED are actionable (INTERNAL uses
	// PENDING/PROCESSING; EXTERNAL Temporal uses PROCESSING after hold).
	// providerReferenceID is stored on success when non-empty.
	MarkTerminal(ctx context.Context, transferID uuid.UUID, succeeded bool, reason, providerReferenceID string) error
	// CancelScheduled cancels a SCHEDULED transfer: sets status CANCELLED with
	// failure_reason CANCELLED and stages a transfer.failed outbox event, all in
	// one transaction. Idempotent: a transfer not in SCHEDULED is a no-op and
	// returns (nil, nil) so both the cancel API and the workflow's own cancel
	// activity can call it safely regardless of which one wins the race.
	CancelScheduled(ctx context.Context, transferID uuid.UUID) (*entities.Transfer, error)
	// SetSettlementSnapshot freezes quoted source/destination amounts, FX rates
	// and fee on the transfer, and advances PENDING → PROCESSING. Used by the
	// Temporal INTERNAL path before Wallet.Move so the transfer row matches the
	// legacy ExecuteInternalTransfer audit fields. Idempotent for non-PENDING.
	SetSettlementSnapshot(
		ctx context.Context,
		transferID uuid.UUID,
		sourceAmount, destinationAmount, sourceRate, destinationRate decimal.Decimal,
		sourceCurrency, destinationCurrency string,
		feeAmount decimal.Decimal,
		feeCurrency string,
	) error
	// ListByUser returns a page of transfers owned by userID, optionally filtered
	// by status and accountRef (nil = no filter). accountRef matches either
	// from_account_ref or to_account_ref.
	ListByUser(
		ctx context.Context,
		userID uuid.UUID,
		status, accountRef *string,
		limit, offset int32,
	) ([]*entities.Transfer, error)
	// CountByUser returns the total number of transfers owned by userID matching
	// the optional status and accountRef filters (nil = no filter).
	CountByUser(ctx context.Context, userID uuid.UUID, status, accountRef *string) (int64, error)
}
