package repositories

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/infrastructure/gen"
)

// PgUUID wraps a uuid.UUID as a pgtype.UUID for query parameters.
func PgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgUUID is the internal alias for backward compatibility.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return PgUUID(id)
}

func accountToEntity(row *gen.Account) *entities.Account {
	return &entities.Account{
		ID:               row.ID.Bytes,
		Ref:              row.AccountRef,
		UserID:           row.UserID.Bytes,
		Name:             row.Name,
		Currency:         row.Currency,
		AvailableBalance: row.AvailableBalance,
		HoldBalance:      row.HoldBalance,
		Status:           entities.AccountStatus(row.Status),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
