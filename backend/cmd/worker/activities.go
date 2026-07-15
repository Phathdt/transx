package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	transferentities "transx/internal/modules/transfer/domain/entities"
	transferinterfaces "transx/internal/modules/transfer/domain/interfaces"
	walletentities "transx/internal/modules/wallet/domain/entities"
	walletinterfaces "transx/internal/modules/wallet/domain/interfaces"
	bankv1 "transx/internal/platform/grpc/gen/bank/v1"
	walletv1 "transx/internal/platform/grpc/gen/wallet/v1"
)

// BusinessErrorType is the Temporal application-error type for permanent
// transfer failures. The workflow treats this as non-retryable and marks the
// transfer FAILED with the encoded reason.
const BusinessErrorType = "TRANSFER_BUSINESS"

// Bank outcome strings returned by BankSubmit/BankQuery activities.
const (
	BankOutcomeSuccess = "SUCCESS"
	BankOutcomeFailure = "FAILURE"
	// BankOutcomeUnknown means the bank call timed out or the outcome is not
	// yet known. The workflow must keep the hold and poll Bank.Query — never
	// terminalize on UNKNOWN alone.
	BankOutcomeUnknown = "UNKNOWN"
)

// Activities holds the Temporal activity methods for the Transfer service.
// Money/bank activities are thin adapters around gRPC clients (Wallet, Bank,
// FX). MarkTerminal / LoadTransfer / PrepareInternalMove talk to transfer (and
// read-only wallet account) repositories in-process because they only touch
// tables the Transfer worker already owns or is allowed to read.
type Activities struct {
	wallet   walletv1.WalletServiceClient
	bank     bankv1.BankServiceClient
	fx       transferinterfaces.FXService
	transfer transferinterfaces.TransferRepository
	accounts walletinterfaces.AccountRepository
}

// NewActivities builds the activity struct passed to worker.RegisterActivity.
// accounts is required for the INTERNAL prepare path (currency/status lookup);
// bank may be nil until the external saga is enabled.
func NewActivities(
	wallet walletv1.WalletServiceClient,
	bank bankv1.BankServiceClient,
	fx transferinterfaces.FXService,
	transfer transferinterfaces.TransferRepository,
	accounts walletinterfaces.AccountRepository,
) *Activities {
	return &Activities{
		wallet:   wallet,
		bank:     bank,
		fx:       fx,
		transfer: transfer,
		accounts: accounts,
	}
}

// businessError wraps a permanent failure reason as a non-retryable Temporal
// application error. The reason string is the failure code stored on the
// transfer (e.g. INSUFFICIENT_FUNDS).
func businessError(reason string) error {
	return temporal.NewNonRetryableApplicationError(reason, BusinessErrorType, nil)
}

// isBusinessError reports whether err (or its cause chain) is a transfer
// business failure that must not be retried by Temporal.
func isBusinessError(err error) bool {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Type() == BusinessErrorType || appErr.NonRetryable()
	}
	return false
}

// businessReason extracts the failure reason from a business application error.
// Falls back to a generic provider-rejected code when the message is empty.
func businessReason(err error) string {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.Message() != "" {
		return appErr.Message()
	}
	return transferentities.FailureProviderRejected
}

// mapWalletMoveError turns Wallet gRPC business failures into non-retryable
// application errors and leaves transient codes as plain errors for retry.
func mapWalletMoveError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	msg := st.Message()
	switch st.Code() {
	case codes.FailedPrecondition:
		switch {
		case strings.Contains(msg, walletinterfaces.ErrInsufficientFunds.Error()):
			return businessError(transferentities.FailureInsufficientFunds)
		case strings.Contains(msg, walletinterfaces.ErrAccountNotActive.Error()):
			return businessError(transferentities.FailureAccountNotActive)
		case strings.Contains(msg, walletinterfaces.ErrCurrencyMismatch.Error()):
			// Currency mismatch after a successful quote means the account
			// currency drifted; treat as destination/source not usable.
			return businessError(transferentities.FailureAccountNotActive)
		default:
			return businessError(transferentities.FailureAccountNotActive)
		}
	case codes.NotFound:
		return businessError(transferentities.FailureAccountNotActive)
	case codes.InvalidArgument:
		return businessError(transferentities.FailureAccountNotActive)
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return err
	default:
		return err
	}
}

// LoadTransferResult is the activity-serializable view of a transfer needed by
// the INTERNAL workflow. Amounts stay as decimal.Decimal (JSON string form).
type LoadTransferResult struct {
	TransferID          uuid.UUID
	TransferType        string
	Status              string
	FromAccountRef      string
	ToAccountRef        string
	TransactionAmount   decimal.Decimal
	TransactionCurrency string
	// AlreadyTerminal is true when status is SUCCEEDED/FAILED so the workflow
	// can exit without re-running money movement.
	AlreadyTerminal bool
}

// LoadTransfer loads the transfer by id. Unknown id returns a business error
// (nothing to process). Terminal statuses return AlreadyTerminal=true.
func (a *Activities) LoadTransfer(ctx context.Context, transferID uuid.UUID) (LoadTransferResult, error) {
	t, err := a.transfer.GetByID(ctx, transferID)
	if err != nil {
		return LoadTransferResult{}, err
	}
	if t == nil {
		return LoadTransferResult{}, businessError(transferentities.FailureAccountNotActive)
	}
	alreadyTerminal := t.Status == transferentities.TransferStatusSucceeded ||
		t.Status == transferentities.TransferStatusFailed
	return LoadTransferResult{
		TransferID:          t.ID,
		TransferType:        t.TransferType,
		Status:              string(t.Status),
		FromAccountRef:      t.FromAccountRef,
		ToAccountRef:        t.ToAccountRef,
		TransactionAmount:   t.TransactionAmount,
		TransactionCurrency: t.TransactionCurrency,
		AlreadyTerminal:     alreadyTerminal,
	}, nil
}

// PrepareInternalMoveInput identifies the INTERNAL transfer to quote and freeze.
type PrepareInternalMoveInput struct {
	TransferID uuid.UUID
}

// PrepareInternalMoveResult carries quoted amounts ready for Wallet.Move plus
// the settlement fields already written on the transfer row.
type PrepareInternalMoveResult struct {
	FromAccountRef      string
	ToAccountRef        string
	SourceAmount        decimal.Decimal
	SourceCurrency      string
	DestinationAmount   decimal.Decimal
	DestinationCurrency string
	SourceFXRate        decimal.Decimal
	DestinationFXRate   decimal.Decimal
	FeeAmount           decimal.Decimal
	FeeCurrency         string
	// AlreadyTerminal is set when the transfer reached SUCCEEDED/FAILED between
	// LoadTransfer and this activity; the workflow must skip Move/MarkTerminal.
	AlreadyTerminal bool
}

// PrepareInternalMove loads both accounts (read-only), quotes FX + fee, freezes
// the settlement snapshot on the transfer (PENDING → PROCESSING), and returns
// the amounts for Wallet.Move. Business failures (missing/inactive accounts, FX
// unavailable) are non-retryable.
func (a *Activities) PrepareInternalMove(
	ctx context.Context,
	input PrepareInternalMoveInput,
) (PrepareInternalMoveResult, error) {
	t, err := a.transfer.GetByID(ctx, input.TransferID)
	if err != nil {
		return PrepareInternalMoveResult{}, err
	}
	if t == nil {
		return PrepareInternalMoveResult{}, businessError(transferentities.FailureAccountNotActive)
	}
	// Already past PENDING: either PROCESSING (retry after snapshot) or terminal.
	// Re-derive quotes from the frozen snapshot when present so Wallet.Move is
	// still callable on activity retry without rewriting settlement.
	if t.Status != transferentities.TransferStatusPending {
		if t.Status == transferentities.TransferStatusProcessing &&
			t.SourceAmount.Valid && t.DestinationAmount.Valid {
			return PrepareInternalMoveResult{
				FromAccountRef:      t.FromAccountRef,
				ToAccountRef:        t.ToAccountRef,
				SourceAmount:        t.SourceAmount.Decimal,
				SourceCurrency:      t.SourceCurrency,
				DestinationAmount:   t.DestinationAmount.Decimal,
				DestinationCurrency: t.DestinationCurrency,
				SourceFXRate:        nullDecimalOrZero(t.SourceFXRate),
				DestinationFXRate:   nullDecimalOrZero(t.DestinationFXRate),
				FeeAmount:           t.FeeAmount,
				FeeCurrency:         t.FeeCurrency,
			}, nil
		}
		if t.Status == transferentities.TransferStatusSucceeded ||
			t.Status == transferentities.TransferStatusFailed {
			// Race: another worker/path already terminalized. Do not surface a
			// business failure (that would try MarkTerminal FAILED with a bogus
			// reason); signal the workflow to exit cleanly.
			return PrepareInternalMoveResult{AlreadyTerminal: true}, nil
		}
		return PrepareInternalMoveResult{}, businessError(transferentities.FailureAccountNotActive)
	}

	from, err := a.accounts.GetByRef(ctx, t.FromAccountRef)
	if err != nil {
		return PrepareInternalMoveResult{}, err
	}
	if from == nil || from.Status != walletentities.AccountStatusActive {
		return PrepareInternalMoveResult{}, businessError(transferentities.FailureAccountNotActive)
	}
	to, err := a.accounts.GetByRef(ctx, t.ToAccountRef)
	if err != nil {
		return PrepareInternalMoveResult{}, err
	}
	if to == nil || to.Status != walletentities.AccountStatusActive {
		return PrepareInternalMoveResult{}, businessError(transferentities.FailureDestNotActive)
	}

	if a.fx == nil {
		return PrepareInternalMoveResult{}, businessError(transferentities.FailureFXRateUnavailable)
	}

	sourceQuote, err := a.fx.Quote(ctx, t.TransactionAmount, t.TransactionCurrency, from.Currency)
	if err != nil {
		if errors.Is(err, transferinterfaces.ErrFXRateUnavailable) {
			return PrepareInternalMoveResult{}, businessError(transferentities.FailureFXRateUnavailable)
		}
		return PrepareInternalMoveResult{}, err
	}
	destinationQuote, err := a.fx.Quote(ctx, t.TransactionAmount, t.TransactionCurrency, to.Currency)
	if err != nil {
		if errors.Is(err, transferinterfaces.ErrFXRateUnavailable) {
			return PrepareInternalMoveResult{}, businessError(transferentities.FailureFXRateUnavailable)
		}
		return PrepareInternalMoveResult{}, err
	}
	feeQuote, err := a.fx.QuoteFee(ctx, t.TransactionCurrency, from.Currency)
	if err != nil {
		if errors.Is(err, transferinterfaces.ErrFXRateUnavailable) {
			return PrepareInternalMoveResult{}, businessError(transferentities.FailureFXRateUnavailable)
		}
		return PrepareInternalMoveResult{}, err
	}

	if err := a.transfer.SetSettlementSnapshot(
		ctx,
		input.TransferID,
		sourceQuote.Amount,
		destinationQuote.Amount,
		sourceQuote.Rate,
		destinationQuote.Rate,
		sourceQuote.Currency,
		destinationQuote.Currency,
		feeQuote.Amount,
		feeQuote.Currency,
	); err != nil {
		return PrepareInternalMoveResult{}, err
	}

	return PrepareInternalMoveResult{
		FromAccountRef:      t.FromAccountRef,
		ToAccountRef:        t.ToAccountRef,
		SourceAmount:        sourceQuote.Amount,
		SourceCurrency:      sourceQuote.Currency,
		DestinationAmount:   destinationQuote.Amount,
		DestinationCurrency: destinationQuote.Currency,
		SourceFXRate:        sourceQuote.Rate,
		DestinationFXRate:   destinationQuote.Rate,
		FeeAmount:           feeQuote.Amount,
		FeeCurrency:         feeQuote.Currency,
	}, nil
}

// PrepareExternalHoldInput identifies the EXTERNAL transfer to validate and freeze.
type PrepareExternalHoldInput struct {
	TransferID uuid.UUID
}

// PrepareExternalHoldResult carries hold parameters after currency check + snapshot.
type PrepareExternalHoldResult struct {
	FromAccountRef  string
	Amount          decimal.Decimal
	Currency        string
	AlreadyTerminal bool
}

// PrepareExternalHold enforces single-currency EXTERNAL (source account currency
// must equal transaction currency), freezes settlement at rate 1, advances
// PENDING → PROCESSING, and returns the hold parameters. Business failures are
// non-retryable.
func (a *Activities) PrepareExternalHold(
	ctx context.Context,
	input PrepareExternalHoldInput,
) (PrepareExternalHoldResult, error) {
	t, err := a.transfer.GetByID(ctx, input.TransferID)
	if err != nil {
		return PrepareExternalHoldResult{}, err
	}
	if t == nil {
		return PrepareExternalHoldResult{}, businessError(transferentities.FailureAccountNotActive)
	}
	if t.Status == transferentities.TransferStatusSucceeded ||
		t.Status == transferentities.TransferStatusFailed {
		return PrepareExternalHoldResult{AlreadyTerminal: true}, nil
	}
	// Retry after snapshot: reuse frozen fields when already PROCESSING/RESERVED.
	if t.Status != transferentities.TransferStatusPending {
		if t.SourceAmount.Valid && t.SourceCurrency != "" {
			return PrepareExternalHoldResult{
				FromAccountRef: t.FromAccountRef,
				Amount:         t.SourceAmount.Decimal,
				Currency:       t.SourceCurrency,
			}, nil
		}
		return PrepareExternalHoldResult{
			FromAccountRef: t.FromAccountRef,
			Amount:         t.TransactionAmount,
			Currency:       t.TransactionCurrency,
		}, nil
	}

	from, err := a.accounts.GetByRef(ctx, t.FromAccountRef)
	if err != nil {
		return PrepareExternalHoldResult{}, err
	}
	if from == nil || from.Status != walletentities.AccountStatusActive {
		return PrepareExternalHoldResult{}, businessError(transferentities.FailureAccountNotActive)
	}
	// EXTERNAL is single-currency: source account must match transaction currency.
	if from.Currency != t.TransactionCurrency {
		return PrepareExternalHoldResult{}, businessError(transferentities.FailureFXRateUnavailable)
	}

	if err := a.transfer.SetSettlementSnapshot(
		ctx,
		input.TransferID,
		t.TransactionAmount,
		decimal.Zero,
		decimal.NewFromInt(1),
		decimal.Zero,
		t.TransactionCurrency,
		"",
		decimal.Zero,
		t.TransactionCurrency,
	); err != nil {
		return PrepareExternalHoldResult{}, err
	}

	return PrepareExternalHoldResult{
		FromAccountRef: t.FromAccountRef,
		Amount:         t.TransactionAmount,
		Currency:       t.TransactionCurrency,
	}, nil
}

func nullDecimalOrZero(nd decimal.NullDecimal) decimal.Decimal {
	if nd.Valid {
		return nd.Decimal
	}
	return decimal.Zero
}

// QuoteFXInput/QuoteFXResult mirror interfaces.FXService.Quote's shape as
// activity-serializable types (Temporal encodes activity args/results as
// JSON, so decimal.Decimal fields marshal as their string form).
type QuoteFXInput struct {
	Amount       decimal.Decimal
	FromCurrency string
	ToCurrency   string
}

type QuoteFXResult struct {
	Amount   decimal.Decimal
	Currency string
	Rate     decimal.Decimal
	Source   string
}

// QuoteFX quotes an amount into the destination currency via the FX gRPC
// service (reached through the existing transfer/infrastructure/fx client).
func (a *Activities) QuoteFX(ctx context.Context, input QuoteFXInput) (QuoteFXResult, error) {
	quote, err := a.fx.Quote(ctx, input.Amount, input.FromCurrency, input.ToCurrency)
	if err != nil {
		if errors.Is(err, transferinterfaces.ErrFXRateUnavailable) {
			return QuoteFXResult{}, businessError(transferentities.FailureFXRateUnavailable)
		}
		return QuoteFXResult{}, err
	}
	return QuoteFXResult{
		Amount:   quote.Amount,
		Currency: quote.Currency,
		Rate:     quote.Rate,
		Source:   quote.Source,
	}, nil
}

// WalletMoveInput/WalletMoveResult mirror wallet.v1.MoveRequest/MoveResponse.
type WalletMoveInput struct {
	TransferID          uuid.UUID
	Operation           string
	FromAccountRef      string
	ToAccountRef        string
	SourceAmount        decimal.Decimal
	SourceCurrency      string
	DestinationAmount   decimal.Decimal
	DestinationCurrency string
	FeeAmount           decimal.Decimal
	FeeCurrency         string
}

type WalletMoveResult struct {
	FromAvailableBalance decimal.Decimal
	ToAvailableBalance   decimal.Decimal
}

// WalletMove calls the Wallet gRPC service's Move RPC. Business failures from
// Wallet (insufficient funds, inactive account, …) become non-retryable
// application errors so Temporal does not hammer a permanent condition.
func (a *Activities) WalletMove(ctx context.Context, input WalletMoveInput) (WalletMoveResult, error) {
	op := input.Operation
	if op == "" {
		op = walletinterfaces.OperationMove
	}
	resp, err := a.wallet.Move(ctx, &walletv1.MoveRequest{
		TransferId:          input.TransferID.String(),
		Operation:           op,
		FromAccountRef:      input.FromAccountRef,
		ToAccountRef:        input.ToAccountRef,
		SourceAmount:        input.SourceAmount.String(),
		SourceCurrency:      input.SourceCurrency,
		DestinationAmount:   input.DestinationAmount.String(),
		DestinationCurrency: input.DestinationCurrency,
		FeeAmount:           input.FeeAmount.String(),
		FeeCurrency:         input.FeeCurrency,
	})
	if err != nil {
		return WalletMoveResult{}, mapWalletMoveError(err)
	}
	fromBalance, err := decimal.NewFromString(resp.GetFromAvailableBalance())
	if err != nil {
		return WalletMoveResult{}, fmt.Errorf("parse from balance: %w", err)
	}
	toBalance, err := decimal.NewFromString(resp.GetToAvailableBalance())
	if err != nil {
		return WalletMoveResult{}, fmt.Errorf("parse to balance: %w", err)
	}
	return WalletMoveResult{FromAvailableBalance: fromBalance, ToAvailableBalance: toBalance}, nil
}

// WalletHoldInput/WalletHoldResult are shared by WalletHold, WalletSettleHold
// and WalletReleaseHold, which all take/return the same shape over the wire.
type WalletHoldInput struct {
	TransferID uuid.UUID
	Operation  string
	AccountRef string
	Amount     decimal.Decimal
	Currency   string
}

type WalletHoldResult struct {
	AvailableBalance decimal.Decimal
	HoldBalance      decimal.Decimal
}

// WalletHold calls the Wallet gRPC service's Hold RPC.
func (a *Activities) WalletHold(ctx context.Context, input WalletHoldInput) (WalletHoldResult, error) {
	resp, err := a.wallet.Hold(ctx, &walletv1.HoldRequest{
		TransferId: input.TransferID.String(),
		Operation:  input.Operation,
		AccountRef: input.AccountRef,
		Amount:     input.Amount.String(),
		Currency:   input.Currency,
	})
	if err != nil {
		return WalletHoldResult{}, mapWalletMoveError(err)
	}
	return walletHoldResultFrom(resp.GetAvailableBalance(), resp.GetHoldBalance())
}

// WalletSettleHold calls the Wallet gRPC service's SettleHold RPC.
func (a *Activities) WalletSettleHold(ctx context.Context, input WalletHoldInput) (WalletHoldResult, error) {
	resp, err := a.wallet.SettleHold(ctx, &walletv1.SettleHoldRequest{
		TransferId: input.TransferID.String(),
		Operation:  input.Operation,
		AccountRef: input.AccountRef,
		Amount:     input.Amount.String(),
		Currency:   input.Currency,
	})
	if err != nil {
		return WalletHoldResult{}, mapWalletMoveError(err)
	}
	return walletHoldResultFrom(resp.GetAvailableBalance(), resp.GetHoldBalance())
}

// WalletReleaseHold calls the Wallet gRPC service's ReleaseHold RPC.
func (a *Activities) WalletReleaseHold(ctx context.Context, input WalletHoldInput) (WalletHoldResult, error) {
	resp, err := a.wallet.ReleaseHold(ctx, &walletv1.ReleaseHoldRequest{
		TransferId: input.TransferID.String(),
		Operation:  input.Operation,
		AccountRef: input.AccountRef,
		Amount:     input.Amount.String(),
		Currency:   input.Currency,
	})
	if err != nil {
		return WalletHoldResult{}, mapWalletMoveError(err)
	}
	return walletHoldResultFrom(resp.GetAvailableBalance(), resp.GetHoldBalance())
}

func walletHoldResultFrom(availableRaw, holdRaw string) (WalletHoldResult, error) {
	available, err := decimal.NewFromString(availableRaw)
	if err != nil {
		return WalletHoldResult{}, err
	}
	hold, err := decimal.NewFromString(holdRaw)
	if err != nil {
		return WalletHoldResult{}, err
	}
	return WalletHoldResult{AvailableBalance: available, HoldBalance: hold}, nil
}

// BankSubmitInput/BankResult mirror bank.v1.SubmitRequest/SubmitResponse.
type BankSubmitInput struct {
	TransferID uuid.UUID
	Amount     decimal.Decimal
	Currency   string
}

type BankResult struct {
	Outcome     string
	ReferenceID string
	Reason      string
}

// BankSubmit calls the Bank gRPC service's Submit RPC. A deadline/timeout is
// mapped to Outcome=UNKNOWN (not an error) so the workflow can keep the hold
// and poll Bank.Query instead of retrying Submit forever.
func (a *Activities) BankSubmit(ctx context.Context, input BankSubmitInput) (BankResult, error) {
	resp, err := a.bank.Submit(ctx, &bankv1.SubmitRequest{
		TransferId: input.TransferID.String(),
		Amount:     input.Amount.String(),
		Currency:   input.Currency,
	})
	if err != nil {
		return mapBankError(err)
	}
	return BankResult{Outcome: resp.GetOutcome(), ReferenceID: resp.GetReferenceId(), Reason: resp.GetReason()}, nil
}

// BankQuery calls the Bank gRPC service's Query RPC. Timeouts map to UNKNOWN
// like Submit so the poll loop can continue without failing the activity.
func (a *Activities) BankQuery(ctx context.Context, transferID uuid.UUID) (BankResult, error) {
	resp, err := a.bank.Query(ctx, &bankv1.QueryRequest{TransferId: transferID.String()})
	if err != nil {
		return mapBankError(err)
	}
	return BankResult{Outcome: resp.GetOutcome(), ReferenceID: resp.GetReferenceId(), Reason: resp.GetReason()}, nil
}

// mapBankError converts bank transport errors: deadlines become UNKNOWN
// outcomes; other codes stay errors for Temporal retry.
func mapBankError(err error) (BankResult, error) {
	st, ok := status.FromError(err)
	if ok && st.Code() == codes.DeadlineExceeded {
		return BankResult{Outcome: BankOutcomeUnknown, Reason: "TIMEOUT"}, nil
	}
	return BankResult{}, err
}

// MarkTerminalInput carries the terminal outcome to record on the transfer.
// Money has already moved via Wallet gRPC (Move / SettleHold / ReleaseHold);
// this activity only advances status + outbox (+ optional provider reference).
type MarkTerminalInput struct {
	TransferID  uuid.UUID
	Succeeded   bool
	ReferenceID string
	Reason      string
}

// MarkTerminal records the terminal transfer outcome in-process (no gRPC hop).
func (a *Activities) MarkTerminal(ctx context.Context, input MarkTerminalInput) error {
	return a.transfer.MarkTerminal(ctx, input.TransferID, input.Succeeded, input.Reason, input.ReferenceID)
}

// BusinessErrorForTest exposes businessError for workflow unit tests without
// re-implementing the Temporal application-error type string.
func BusinessErrorForTest(reason string) error {
	return businessError(reason)
}
