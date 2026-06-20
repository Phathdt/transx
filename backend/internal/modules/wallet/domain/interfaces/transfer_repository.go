package interfaces

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/wallet/domain/entities"
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
	ExecuteInternalTransfer(ctx context.Context, transferID uuid.UUID) error
	// ReserveExternalTransfer moves the amount from available to hold and stages
	// the provider-request outbox event in one transaction (PENDING → RESERVED).
	// Idempotent: a transfer not in PENDING is skipped.
	ReserveExternalTransfer(ctx context.Context, transferID uuid.UUID) error
	// SettleExternalTransfer applies the provider outcome in one transaction
	// (RESERVED → SUCCEEDED on success, → FAILED with hold released on failure).
	// Idempotent: a transfer not in RESERVED is skipped.
	SettleExternalTransfer(ctx context.Context, transferID uuid.UUID, result entities.ProviderResult) error
}
