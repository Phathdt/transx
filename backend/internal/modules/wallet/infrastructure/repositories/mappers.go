package repositories

import (
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/infrastructure/gen"
)

func accountToEntity(row *gen.Account) *entities.Account {
	return &entities.Account{
		ID:               row.ID,
		Ref:              row.AccountRef,
		UserID:           row.UserID,
		Name:             row.Name,
		Currency:         row.Currency,
		AvailableBalance: row.AvailableBalance,
		HoldBalance:      row.HoldBalance,
		Status:           entities.AccountStatus(row.Status),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
