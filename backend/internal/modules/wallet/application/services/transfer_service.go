package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oklog/ulid/v2"
	"github.com/shopspring/decimal"

	"transx/internal/common/apperror"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/modules/wallet/domain/interfaces"
)

// maxAmountScale bounds the number of decimal places on a transfer amount to
// match the NUMERIC(20,4) column; finer precision is rejected rather than
// silently rounded.
const maxAmountScale = 4

// maxIntegerDigits bounds the integer part of an amount to match NUMERIC(20,4)
// (20 total digits − 4 fractional). Larger values would overflow the column and
// surface as a 500; reject them as a 400 at the boundary instead.
const maxIntegerDigits = 16

// pgUniqueViolation is the SQLSTATE for a unique constraint violation.
const pgUniqueViolation = "23505"

// transferTypeInternal and transferTypeExternal are the supported transfer
// types. INTERNAL moves funds between two in-ledger accounts; EXTERNAL sends
// funds out through a provider and has no in-ledger destination.
const (
	transferTypeInternal = "INTERNAL"
	transferTypeExternal = "EXTERNAL"
)

// refPrefix maps a transfer type to its business-reference prefix so the
// reference itself signals whether a transfer is external or internal.
func refPrefix(transferType string) string {
	if transferType == transferTypeExternal {
		return "ETN-"
	}
	return "ITN-"
}

// NewTransferReference builds a business reference: prefix + ULID. The ULID is
// generated at the application layer (time + entropy), independent of the
// DB-assigned UUID primary key.
func NewTransferReference(transferType string) string {
	return refPrefix(transferType) + ulid.Make().String()
}

// transferReferencePattern matches a business reference: an ETN-/ITN- prefix
// followed by a 26-char Crockford base32 ULID (the alphabet excludes I, L, O, U).
var transferReferencePattern = regexp.MustCompile(`^(ETN|ITN)-[0-9A-HJKMNP-TV-Z]{26}$`)

// TransferService creates transfers with authorization and idempotency, and
// reads them back owner-scoped.
type TransferService struct {
	transfers    interfaces.TransferRepository
	accounts     interfaces.AccountRepository
	providerName string
}

func NewTransferService(
	transfers interfaces.TransferRepository,
	accounts interfaces.AccountRepository,
	providerName string,
) *TransferService {
	return &TransferService{transfers: transfers, accounts: accounts, providerName: providerName}
}

// CreateTransfer validates, authorizes and idempotently creates a transfer in
// PENDING state. INTERNAL transfers require an in-ledger destination; EXTERNAL
// transfers send funds out through the configured provider and carry no
// destination. The actual money movement happens asynchronously in the workers.
func (s *TransferService) CreateTransfer(
	ctx context.Context,
	userID uuid.UUID,
	key string,
	cmd dto.CreateTransferCommand,
) (*dto.TransferResponse, error) {
	// The idempotency key is client-supplied so a retry of the same logical
	// request reuses it and replays the original transfer instead of creating a
	// new one. It must be a UUID (uuidv7 recommended for time-ordering); a server
	// could not generate it without defeating retry-safety.
	if key == "" {
		return nil, apperror.NewBadRequestError("missing Idempotency-Key")
	}
	if _, err := uuid.Parse(key); err != nil {
		return nil, apperror.NewBadRequestError("Idempotency-Key must be a UUID")
	}

	tType := transferType(cmd.TransferType)
	fromRef, amount, err := parseTransferCommon(cmd)
	if err != nil {
		return nil, err
	}
	currency := normalizeCurrency(cmd.Currency)
	if !isSupportedCurrency(currency) {
		return nil, apperror.NewBadRequestError("unsupported currency")
	}

	if tType == transferTypeExternal {
		return s.createExternal(ctx, userID, key, fromRef, amount, currency, cmd.ToAccountRef)
	}
	return s.createInternal(ctx, userID, key, fromRef, cmd.ToAccountRef, amount, currency)
}

// createInternal handles an INTERNAL transfer: it requires a destination account
// and validates both accounts belong-to/are-active for the caller.
func (s *TransferService) createInternal(
	ctx context.Context,
	userID uuid.UUID,
	key string,
	fromRef string,
	toRef string,
	amount decimal.Decimal,
	currency string,
) (*dto.TransferResponse, error) {
	// An INTERNAL destination must be an in-system account ref; a free-text id is
	// only valid for EXTERNAL transfers.
	if !accountReferencePattern.MatchString(toRef) {
		return nil, apperror.NewBadRequestError("invalid toAccountRef")
	}
	if fromRef == toRef {
		return nil, apperror.NewBadRequestError("from and to accounts must differ")
	}

	hash := requestHash(fromRef, toRef, amount, currency, transferTypeInternal, "")

	if existing, err := s.transfers.FindByUserAndKey(ctx, userID, key); err != nil {
		return nil, err
	} else if existing != nil {
		return idempotentResult(existing, hash)
	}

	// Authorization (P2P): the source account must belong to the caller. The
	// destination may be someone else's. This is the primary theft guard.
	from, err := s.accounts.GetByRef(ctx, fromRef)
	if err != nil {
		return nil, err
	}
	if from == nil || from.UserID != userID {
		return nil, apperror.NewForbiddenError("from account does not belong to caller")
	}
	to, err := s.accounts.GetByRef(ctx, toRef)
	if err != nil {
		return nil, err
	}
	if to == nil {
		return nil, apperror.NewBadRequestError("to account not found")
	}

	if !from.IsActive() || !to.IsActive() {
		return nil, apperror.NewUnprocessableError("account not active")
	}
	return s.persist(ctx, &entities.Transfer{
		FromAccountRef:      fromRef,
		ToAccountRef:        toRef,
		TransactionAmount:   amount,
		TransactionCurrency: currency,
		FeeAmount:           decimal.Zero,
		FeeCurrency:         currency,
		TransferType:        transferTypeInternal,
		Reference:           NewTransferReference(transferTypeInternal),
		Status:              entities.TransferStatusPending,
		UserID:              userID,
		IdempotencyKey:      key,
		RequestHash:         hash,
	}, userID, key, hash)
}

// createExternal handles an EXTERNAL transfer: there is no in-ledger
// destination account, the provider is stamped from config, and the reference id
// is filled in later at settle time. The destination, when supplied, is a
// free-text beneficiary id that is stored as-is (no in-system validation).
func (s *TransferService) createExternal(
	ctx context.Context,
	userID uuid.UUID,
	key string,
	fromRef string,
	amount decimal.Decimal,
	currency string,
	toRef string,
) (*dto.TransferResponse, error) {
	hash := requestHash(fromRef, toRef, amount, currency, transferTypeExternal, s.providerName)

	if existing, err := s.transfers.FindByUserAndKey(ctx, userID, key); err != nil {
		return nil, err
	} else if existing != nil {
		return idempotentResult(existing, hash)
	}

	from, err := s.accounts.GetByRef(ctx, fromRef)
	if err != nil {
		return nil, err
	}
	if from == nil || from.UserID != userID {
		return nil, apperror.NewForbiddenError("from account does not belong to caller")
	}
	if !from.IsActive() {
		return nil, apperror.NewUnprocessableError("account not active")
	}
	if from.Currency != currency {
		return nil, apperror.NewUnprocessableError("currency mismatch")
	}

	return s.persist(ctx, &entities.Transfer{
		FromAccountRef:      fromRef,
		ToAccountRef:        toRef, // free-text beneficiary or empty; no in-ledger destination.
		TransactionAmount:   amount,
		TransactionCurrency: currency,
		FeeAmount:           decimal.Zero,
		FeeCurrency:         currency,
		TransferType:        transferTypeExternal,
		Reference:           NewTransferReference(transferTypeExternal),
		Provider:            s.providerName,
		Status:              entities.TransferStatusPending,
		UserID:              userID,
		IdempotencyKey:      key,
		RequestHash:         hash,
	}, userID, key, hash)
}

// persist creates the transfer, applying the idempotency replay/conflict rule on
// a unique-violation race.
func (s *TransferService) persist(
	ctx context.Context,
	t *entities.Transfer,
	userID uuid.UUID,
	key, hash string,
) (*dto.TransferResponse, error) {
	created, err := s.transfers.Create(ctx, t)
	if err != nil {
		// Idempotency race: a concurrent request with the same key won the unique
		// index. Re-read and apply the same replay/conflict rule instead of 500.
		// This assumes the violated index is the (user_id, idempotency_key) one; a
		// reference collision is ~80-bit-entropy improbable and would fall through
		// to a 500, which is acceptable for that practically-impossible case.
		if isUniqueViolation(err) {
			existing, ferr := s.transfers.FindByUserAndKey(ctx, userID, key)
			if ferr != nil {
				return nil, ferr
			}
			if existing != nil {
				return idempotentResult(existing, hash)
			}
		}
		return nil, err
	}
	return transferToResponse(created), nil
}

// GetTransfer returns the caller's transfer by its business reference
// (ETN-/ITN- + ULID); one belonging to another user is reported as not found.
// A malformed reference is a 400 so a junk id is distinguishable from a
// well-formed id that simply does not exist (404).
func (s *TransferService) GetTransfer(
	ctx context.Context,
	reference string,
	userID uuid.UUID,
) (*dto.TransferResponse, error) {
	if !transferReferencePattern.MatchString(reference) {
		return nil, apperror.NewBadRequestError("invalid transferId")
	}
	transfer, err := s.transfers.GetByReferenceForUser(ctx, reference, userID)
	if err != nil {
		return nil, err
	}
	if transfer == nil {
		return nil, apperror.NewNotFoundError("transfer not found")
	}
	return transferToResponse(transfer), nil
}

// parseTransferCommon validates and parses the fields shared by both transfer
// types: the source account ref and the amount. The source is always an
// in-system account, so its ref must match the ACC- pattern. Destination
// handling differs per type and is validated in the type-specific branch.
func parseTransferCommon(
	cmd dto.CreateTransferCommand,
) (fromRef string, amount decimal.Decimal, err error) {
	fromRef = cmd.FromAccountRef
	if !accountReferencePattern.MatchString(fromRef) {
		return "", decimal.Zero, apperror.NewBadRequestError("invalid fromAccountRef")
	}
	amount, perr := decimal.NewFromString(cmd.Amount)
	if perr != nil {
		return "", decimal.Zero, apperror.NewBadRequestError("invalid amount")
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return "", decimal.Zero, apperror.NewBadRequestError("amount must be positive")
	}
	if amount.Exponent() < -maxAmountScale {
		return "", decimal.Zero, apperror.NewBadRequestError("amount has too many decimal places")
	}
	// Integer digits = total significant digits minus the fractional scale.
	if amount.NumDigits()+int(amount.Exponent()) > maxIntegerDigits {
		return "", decimal.Zero, apperror.NewBadRequestError("amount too large")
	}
	return fromRef, amount, nil
}

// transferType defaults to INTERNAL when unset.
func transferType(t string) string {
	if t == "" {
		return transferTypeInternal
	}
	return t
}

// requestHash is a canonical hash of the idempotency-relevant fields so reusing
// a key with a different body can be detected and rejected. toRef is empty for
// EXTERNAL transfers with no destination; provider is empty for INTERNAL — both
// feed the hash so the same key cannot be replayed across differing transfer
// shapes. Account refs are stable, so the hash is stable across the UUID split.
func requestHash(fromRef, toRef string, amount decimal.Decimal, currency, tType, provider string) string {
	canonical := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		fromRef, toRef, amount.String(), currency, tType, provider)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// idempotentResult replays a prior transfer when the request body matches, or
// returns 409 when the same key was reused with a different body.
func idempotentResult(existing *entities.Transfer, hash string) (*dto.TransferResponse, error) {
	if existing.RequestHash != hash {
		return nil, apperror.NewConflictError("idempotency key reused with a different request")
	}
	return transferToResponse(existing), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

func transferToResponse(t *entities.Transfer) *dto.TransferResponse {
	return &dto.TransferResponse{
		TransferID:          t.Reference,
		Status:              string(t.Status),
		TransactionAmount:   t.TransactionAmount.String(),
		TransactionCurrency: t.TransactionCurrency,
		SourceAmount:        decimalString(t.SourceAmount),
		SourceCurrency:      t.SourceCurrency,
		DestinationAmount:   decimalString(t.DestinationAmount),
		DestinationCurrency: t.DestinationCurrency,
		SourceFXRate:        decimalString(t.SourceFXRate),
		DestinationFXRate:   decimalString(t.DestinationFXRate),
		FeeAmount:           t.FeeAmount.String(),
		FeeCurrency:         t.FeeCurrency,
		FailureReason:       t.FailureReason,
	}
}

func decimalString(value decimal.NullDecimal) string {
	if !value.Valid {
		return ""
	}
	return value.Decimal.String()
}
