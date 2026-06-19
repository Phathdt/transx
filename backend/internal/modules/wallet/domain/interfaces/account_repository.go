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
	// GetByIDForUser scopes the read to the owner so a caller cannot read
	// another user's account (prevents IDOR on GET).
	GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (*entities.Account, error)
}
