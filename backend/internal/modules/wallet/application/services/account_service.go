package services

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
)

// NewAccountReference builds an account's external business id: ACC- + ULID.
// The ULID is generated at the application layer (time + entropy), independent
// of the DB-assigned UUID primary key, mirroring the transfer reference scheme.
func NewAccountReference() string {
	return "ACC-" + ulid.Make().String()
}

// accountReferencePattern matches an account ref: an ACC- prefix followed by a
// 26-char Crockford base32 ULID (the alphabet excludes I, L, O, U).
var accountReferencePattern = regexp.MustCompile(`^ACC-[0-9A-HJKMNP-TV-Z]{26}$`)

// externalReferencePattern bounds an external beneficiary ref before it is
// concatenated into the unauthenticated outbound provider lookup path, so a
// caller cannot smuggle path segments (e.g. encoded slashes) to the upstream.
var externalReferencePattern = regexp.MustCompile(`^EXT-[A-Za-z0-9-]{1,64}$`)

// AccountService handles account creation and owner-scoped reads.
type AccountService struct {
	accounts       interfaces.AccountRepository
	providerLookup interfaces.ProviderAccountLookupClient
}

func NewAccountService(
	accounts interfaces.AccountRepository,
	providerLookups ...interfaces.ProviderAccountLookupClient,
) *AccountService {
	var providerLookup interfaces.ProviderAccountLookupClient
	if len(providerLookups) > 0 {
		providerLookup = providerLookups[0]
	}
	return &AccountService{accounts: accounts, providerLookup: providerLookup}
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
		Ref:      NewAccountReference(),
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

// GetAccount returns the caller's account by its external ref. Reads are
// owner-scoped: an account that does not belong to the caller is reported as not
// found to avoid leaking its existence. A malformed ref is a 400 so a junk id is
// distinguishable from a well-formed ref that simply does not exist (404).
func (s *AccountService) GetAccount(
	ctx context.Context,
	ref string,
	userID uuid.UUID,
) (*dto.AccountResponse, error) {
	if !accountReferencePattern.MatchString(ref) {
		return nil, apperror.NewBadRequestError("invalid accountRef")
	}
	account, err := s.accounts.GetByRefForUser(ctx, ref, userID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, apperror.NewNotFoundError("account not found")
	}
	return accountToResponse(account), nil
}

func (s *AccountService) LookupAccount(
	ctx context.Context,
	accountType string,
	ref string,
) (*dto.AccountLookupResponse, error) {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case "internal":
		return s.LookupInternalAccount(ctx, ref)
	case "external":
		return s.LookupExternalAccount(ctx, ref)
	default:
		return nil, apperror.NewBadRequestError("unsupported accountType")
	}
}

// LookupInternalAccount resolves any in-system account by its ref so a caller can
// validate a transfer recipient they don't own. It is not owner-scoped; the route
// is still authenticated and the compact view leaks no balances or identities.
func (s *AccountService) LookupInternalAccount(
	ctx context.Context,
	ref string,
) (*dto.AccountLookupResponse, error) {
	if !accountReferencePattern.MatchString(ref) {
		return nil, apperror.NewBadRequestError("invalid accountRef")
	}
	lookup, err := s.accounts.GetLookupByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if lookup == nil {
		return nil, apperror.NewNotFoundError("account not found")
	}
	return accountLookupToResponse(lookup), nil
}

func (s *AccountService) LookupExternalAccount(
	ctx context.Context,
	ref string,
) (*dto.AccountLookupResponse, error) {
	if !externalReferencePattern.MatchString(ref) {
		return nil, apperror.NewBadRequestError("invalid accountRef")
	}
	if s.providerLookup == nil {
		return nil, apperror.NewBadGatewayError("provider lookup unavailable", nil)
	}
	lookup, err := s.providerLookup.LookupAccount(ctx, ref)
	if err != nil {
		return nil, err
	}
	if lookup == nil {
		return nil, apperror.NewNotFoundError("account not found")
	}
	return accountLookupToResponse(lookup), nil
}

func accountToResponse(a *entities.Account) *dto.AccountResponse {
	return &dto.AccountResponse{
		AccountRef:       a.Ref,
		AvailableBalance: a.AvailableBalance.String(),
		HoldBalance:      a.HoldBalance.String(),
		Currency:         a.Currency,
		Status:           string(a.Status),
	}
}

func accountLookupToResponse(a *entities.AccountLookup) *dto.AccountLookupResponse {
	return &dto.AccountLookupResponse{
		AccountRef: a.AccountRef,
		Currency:   a.Currency,
		Status:     a.Status,
		HolderName: a.HolderName,
	}
}
