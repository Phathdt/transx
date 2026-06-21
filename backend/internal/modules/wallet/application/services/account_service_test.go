package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/testmocks"
)

func TestAccountServiceCreateAccount(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("creates account with supported currency", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(a *entities.Account) bool {
				return a.UserID == userID &&
					a.Name == "My USD Account" &&
					a.Currency == "USD" &&
					a.Status == entities.AccountStatusActive
			})).
			RunAndReturn(func(ctx context.Context, a *entities.Account) (*entities.Account, error) {
				a.ID = uuid.New()
				a.AvailableBalance = decimal.NewFromInt(0)
				a.HoldBalance = decimal.NewFromInt(0)
				return a, nil
			})

		service := NewAccountService(accountRepo)

		result, err := service.CreateAccount(ctx, userID, dto.CreateAccountCommand{
			Currency: "USD",
			Name:     "My USD Account",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, result.AccountRef)
		assert.Regexp(t, `^ACC-[0-9A-HJKMNP-TV-Z]{26}$`, result.AccountRef)
		assert.Equal(t, "USD", result.Currency)
		assert.Equal(t, string(entities.AccountStatusActive), result.Status)
		assert.Equal(t, "0", result.AvailableBalance)
		assert.Equal(t, "0", result.HoldBalance)
	})

	t.Run("unsupported currency returns bad request", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo)

		result, err := service.CreateAccount(ctx, userID, dto.CreateAccountCommand{
			Currency: "XYZ",
			Name:     "Invalid Account",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("normalizes currency to uppercase", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().
			Create(ctx, mock.MatchedBy(func(a *entities.Account) bool {
				return a.Currency == "EUR"
			})).
			RunAndReturn(func(ctx context.Context, a *entities.Account) (*entities.Account, error) {
				a.ID = uuid.New()
				a.AvailableBalance = decimal.NewFromInt(0)
				a.HoldBalance = decimal.NewFromInt(0)
				return a, nil
			})

		service := NewAccountService(accountRepo)

		result, err := service.CreateAccount(ctx, userID, dto.CreateAccountCommand{
			Currency: "eur",
			Name:     "EUR Account",
		})

		require.NoError(t, err)
		assert.Equal(t, "EUR", result.Currency)
	})
}

func TestAccountServiceGetAccount(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	accountID := uuid.New()
	accountRef := NewAccountReference()

	t.Run("returns account for owner", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		account := &entities.Account{
			ID:               accountID,
			Ref:              accountRef,
			UserID:           userID,
			Name:             "My Account",
			Currency:         "USD",
			Status:           entities.AccountStatusActive,
			AvailableBalance: decimal.NewFromInt(1000),
			HoldBalance:      decimal.NewFromInt(0),
		}
		accountRepo.EXPECT().
			GetByRefForUser(ctx, accountRef, userID).
			Return(account, nil)

		service := NewAccountService(accountRepo)

		result, err := service.GetAccount(ctx, accountRef, userID)

		require.NoError(t, err)
		assert.Equal(t, accountRef, result.AccountRef)
		assert.Equal(t, "USD", result.Currency)
		assert.Equal(t, "1000", result.AvailableBalance)
	})

	t.Run("malformed ref returns bad request", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo)

		result, err := service.GetAccount(ctx, "not-a-ref", userID)

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("account not found for different owner", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().
			GetByRefForUser(ctx, accountRef, userID).
			Return(nil, nil)

		service := NewAccountService(accountRepo)

		result, err := service.GetAccount(ctx, accountRef, userID)

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 404, appErr.Status)
	})

	t.Run("returns not found on error", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		differentUserID := uuid.New()
		accountRepo.EXPECT().
			GetByRefForUser(ctx, accountRef, differentUserID).
			Return(nil, nil)

		service := NewAccountService(accountRepo)

		result, err := service.GetAccount(ctx, accountRef, differentUserID)

		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestAccountServiceLookupAccount(t *testing.T) {
	ctx := context.Background()
	accountRef := NewAccountReference()

	t.Run("internal lookup returns compact recipient view", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, accountRef).
			Return(&entities.AccountLookup{
				AccountRef: accountRef,
				Currency:   "USD",
				Status:     string(entities.AccountStatusActive),
				HolderName: "Alice Tran",
			}, nil)

		service := NewAccountService(accountRepo)

		result, err := service.LookupAccount(ctx, "internal", accountRef)

		require.NoError(t, err)
		assert.Equal(t, accountRef, result.AccountRef)
		assert.Equal(t, "USD", result.Currency)
		assert.Equal(t, string(entities.AccountStatusActive), result.Status)
		assert.Equal(t, "Alice Tran", result.HolderName)
	})

	t.Run("internal malformed ref returns bad request", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo)

		result, err := service.LookupAccount(ctx, "internal", "bad-ref")

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("internal unknown ref returns not found", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().
			GetLookupByRef(ctx, accountRef).
			Return(nil, nil)
		service := NewAccountService(accountRepo)

		result, err := service.LookupAccount(ctx, "internal", accountRef)

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 404, appErr.Status)
	})

	t.Run("external lookup delegates to provider", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo, fakeAccountLookupClient{
			lookup: &entities.AccountLookup{
				AccountRef: "EXT-ACME-USD-001",
				Currency:   "USD",
				Status:     string(entities.AccountStatusActive),
				HolderName: "Acme Treasury",
			},
		})

		result, err := service.LookupAccount(ctx, "external", "EXT-ACME-USD-001")

		require.NoError(t, err)
		assert.Equal(t, "EXT-ACME-USD-001", result.AccountRef)
		assert.Equal(t, "Acme Treasury", result.HolderName)
	})

	t.Run("external unknown ref returns not found", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo, fakeAccountLookupClient{})

		result, err := service.LookupAccount(ctx, "external", "EXT-MISSING")

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 404, appErr.Status)
	})

	t.Run("external blank ref returns bad request", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo, fakeAccountLookupClient{})

		result, err := service.LookupAccount(ctx, "external", " ")

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})

	t.Run("external ref with path-smuggling characters returns bad request", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo, fakeAccountLookupClient{})

		for _, ref := range []string{"EXT-../internal/ACC-1", "EXT-x%2fy", "ACC-1", "EXT-" + strings.Repeat("x", 65)} {
			result, err := service.LookupAccount(ctx, "external", ref)

			require.Error(t, err)
			assert.Nil(t, result)
			appErr, ok := err.(*apperror.AppError)
			require.True(t, ok)
			assert.Equal(t, 400, appErr.Status, "ref %q must be rejected before reaching provider", ref)
		}
	})

	t.Run("external without provider returns bad gateway", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo)

		result, err := service.LookupAccount(ctx, "external", "EXT-ACME-USD-001")

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 502, appErr.Status)
	})

	t.Run("external provider error propagates", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo, fakeAccountLookupClient{err: assert.AnError})

		result, err := service.LookupAccount(ctx, "external", "EXT-ACME-USD-001")

		assert.Nil(t, result)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("unsupported type returns bad request", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		service := NewAccountService(accountRepo)

		result, err := service.LookupAccount(ctx, "partner", accountRef)

		require.Error(t, err)
		assert.Nil(t, result)
		appErr, ok := err.(*apperror.AppError)
		assert.True(t, ok)
		assert.Equal(t, 400, appErr.Status)
	})
}

func TestAccountServiceRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	accountRef := NewAccountReference()
	wantErr := assert.AnError

	t.Run("create returns repository error", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, wantErr)
		service := NewAccountService(accountRepo)

		result, err := service.CreateAccount(ctx, userID, dto.CreateAccountCommand{Currency: "USD", Name: "USD"})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("get returns repository error", func(t *testing.T) {
		accountRepo := testmocks.NewAccountRepository(t)
		accountRepo.EXPECT().GetByRefForUser(ctx, accountRef, userID).Return(nil, wantErr)
		service := NewAccountService(accountRepo)

		result, err := service.GetAccount(ctx, accountRef, userID)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, wantErr)
	})
}

type fakeAccountLookupClient struct {
	lookup *entities.AccountLookup
	err    error
}

func (f fakeAccountLookupClient) LookupAccount(
	_ context.Context,
	_ string,
) (*entities.AccountLookup, error) {
	return f.lookup, f.err
}
