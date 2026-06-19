package interfaces

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/modules/auth/domain/entities"
)

// UserRepository loads users for authentication. Auth only reads users; account
// creation is out of scope for this service.
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*entities.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*entities.User, error)
}
