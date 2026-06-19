package services

import (
	"context"

	"github.com/google/uuid"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
)

// AccountService handles account creation and owner-scoped reads.
type AccountService struct {
	accounts interfaces.AccountRepository
}

func NewAccountService(accounts interfaces.AccountRepository) *AccountService {
	return &AccountService{accounts: accounts}
}

// CreateAccount opens a new wallet for the caller in a supported currency.
func (s *AccountService) CreateAccount(
	ctx context.Context,
	userID uuid.UUID,
	cmd dto.CreateAccountCommand,
) (*dto.AccountResponse, error) {
	if !isSupportedCurrency(cmd.Currency) {
		return nil, apperror.NewBadRequestError("unsupported currency")
	}

	created, err := s.accounts.Create(ctx, &entities.Account{
		UserID:   userID,
		Name:     cmd.Name,
		Currency: normalizeCurrency(cmd.Currency),
		Status:   entities.AccountStatusActive,
	})
	if err != nil {
		return nil, err
	}
	return accountToResponse(created), nil
}

// GetAccount returns the caller's account. Reads are owner-scoped: an account
// that does not belong to the caller is reported as not found to avoid leaking
// its existence.
func (s *AccountService) GetAccount(
	ctx context.Context,
	id, userID uuid.UUID,
) (*dto.AccountResponse, error) {
	account, err := s.accounts.GetByIDForUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, apperror.NewNotFoundError("account not found")
	}
	return accountToResponse(account), nil
}

func accountToResponse(a *entities.Account) *dto.AccountResponse {
	return &dto.AccountResponse{
		AccountID:        a.ID.String(),
		AvailableBalance: a.AvailableBalance.String(),
		HoldBalance:      a.HoldBalance.String(),
		Currency:         a.Currency,
		Status:           string(a.Status),
	}
}
