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
}
