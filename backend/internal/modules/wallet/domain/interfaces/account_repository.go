package interfaces

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/wallet/domain/entities"
)

// AccountRepository persists and loads wallet accounts.
type AccountRepository interface {
	Create(ctx context.Context, a *entities.Account) (*entities.Account, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entities.Account, error)
	// GetByRef loads an account by its external ref (ACC- + ULID), unscoped.
	// Used for transfer authorization where the caller validates ownership.
	GetByRef(ctx context.Context, ref string) (*entities.Account, error)
	// GetByRefForUser scopes the read to the owner so a caller cannot read
	// another user's account (prevents IDOR on GET).
	GetByRefForUser(ctx context.Context, ref string, userID uuid.UUID) (*entities.Account, error)
	// GetLookupByRef returns the compact transfer-lookup view for any in-system
	// account, not owner-scoped: a caller validates a recipient they don't own
	// before an internal transfer. HolderName comes from the owner identity, not
	// the wallet account label.
	GetLookupByRef(ctx context.Context, ref string) (*entities.AccountLookup, error)
	// ListByUser returns a page of accounts owned by userID, optionally filtered
	// by currency and status (nil = no filter).
	ListByUser(
		ctx context.Context,
		userID uuid.UUID,
		currency, status *string,
		limit, offset int32,
	) ([]*entities.Account, error)
	// CountByUser returns the total number of accounts owned by userID matching
	// the optional currency and status filters (nil = no filter).
	CountByUser(ctx context.Context, userID uuid.UUID, currency, status *string) (int64, error)
}
