package interfaces

import (
	"context"

	"github.com/google/uuid"

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
