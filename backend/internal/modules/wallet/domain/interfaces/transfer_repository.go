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
	// GetByIDForUser returns the transfer only when it belongs to the caller.
	GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (*entities.Transfer, error)
	// FindByUserAndKey looks up a transfer by its idempotency key, scoped to the
	// owner. Returns (nil, nil) when none exists.
	FindByUserAndKey(ctx context.Context, userID uuid.UUID, key string) (*entities.Transfer, error)
	// ExecuteInternalTransfer runs the debit/credit/ledger/status/outbox steps
	// for one transfer atomically in a single transaction. It is idempotent: a
	// transfer not in PENDING is skipped.
	ExecuteInternalTransfer(ctx context.Context, transferID uuid.UUID) error
}
