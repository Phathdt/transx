package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/modules/wallet/infrastructure/gen"
)

// PostgresAccountRepository implements interfaces.AccountRepository on top of the
// sqlc-generated queries.
type PostgresAccountRepository struct {
	q *gen.Queries
}

func NewPostgresAccountRepository(q *gen.Queries) *PostgresAccountRepository {
	return &PostgresAccountRepository{q: q}
}

var _ interfaces.AccountRepository = (*PostgresAccountRepository)(nil)

func (r *PostgresAccountRepository) Create(
	ctx context.Context,
	a *entities.Account,
) (*entities.Account, error) {
	row, err := r.q.CreateAccount(ctx, gen.CreateAccountParams{
		UserID:           pgUUID(a.UserID),
		Name:             a.Name,
		Currency:         a.Currency,
		AvailableBalance: a.AvailableBalance,
		HoldBalance:      a.HoldBalance,
		Status:           string(a.Status),
		AccountRef:       a.Ref,
	})
	if err != nil {
		return nil, err
	}
	return accountToEntity(row), nil
}

func (r *PostgresAccountRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*entities.Account, error) {
	row, err := r.q.GetAccountByID(ctx, pgUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return accountToEntity(row), nil
}

func (r *PostgresAccountRepository) GetByRef(
	ctx context.Context,
	ref string,
) (*entities.Account, error) {
	row, err := r.q.GetAccountByRef(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return accountToEntity(row), nil
}

func (r *PostgresAccountRepository) GetByRefForUser(
	ctx context.Context,
	ref string,
	userID uuid.UUID,
) (*entities.Account, error) {
	row, err := r.q.GetAccountByRefForUser(ctx, gen.GetAccountByRefForUserParams{
		AccountRef: ref,
		UserID:     pgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return accountToEntity(row), nil
}

func (r *PostgresAccountRepository) GetLookupByRef(
	ctx context.Context,
	ref string,
) (*entities.AccountLookup, error) {
	row, err := r.q.GetAccountLookupByRef(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &entities.AccountLookup{
		AccountRef: row.AccountRef,
		Currency:   row.Currency,
		Status:     row.Status,
		HolderName: row.HolderName,
	}, nil
}

func (r *PostgresAccountRepository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	currency, status *string,
	limit, offset int32,
) ([]*entities.Account, error) {
	rows, err := r.q.ListAccountsByUser(ctx, gen.ListAccountsByUserParams{
		UserID:   pgUUID(userID),
		Currency: currency,
		Status:   status,
		Lim:      limit,
		Off:      offset,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*entities.Account, len(rows))
	for i, row := range rows {
		result[i] = accountToEntity(row)
	}
	return result, nil
}

func (r *PostgresAccountRepository) CountByUser(
	ctx context.Context,
	userID uuid.UUID,
	currency, status *string,
) (int64, error) {
	return r.q.CountAccountsByUser(ctx, gen.CountAccountsByUserParams{
		UserID:   pgUUID(userID),
		Currency: currency,
		Status:   status,
	})
}
