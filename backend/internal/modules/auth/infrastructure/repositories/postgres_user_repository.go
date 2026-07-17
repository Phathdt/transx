package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"transx/internal/modules/auth/domain/entities"
	"transx/internal/modules/auth/domain/interfaces"
	"transx/internal/modules/auth/infrastructure/gen"
)

// PostgresUserRepository implements interfaces.UserRepository on top of the
// sqlc-generated queries.
type PostgresUserRepository struct {
	q *gen.Queries
}

func NewPostgresUserRepository(q *gen.Queries) *PostgresUserRepository {
	return &PostgresUserRepository{q: q}
}

var _ interfaces.UserRepository = (*PostgresUserRepository)(nil)

func (r *PostgresUserRepository) FindByEmail(ctx context.Context, email string) (*entities.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toEntity(row), nil
}

func (r *PostgresUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*entities.User, error) {
	row, err := r.q.GetUserByID(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toEntity(row), nil
}

func toEntity(row *gen.User) *entities.User {
	return &entities.User{
		ID:           row.ID.Bytes,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		Name:         row.Name,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
