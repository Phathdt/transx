package worker

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	transferentities "transx/internal/modules/transfer/domain/entities"
	transferinterfaces "transx/internal/modules/transfer/domain/interfaces"
	bankv1 "transx/internal/platform/grpc/gen/bank/v1"
	walletv1 "transx/internal/platform/grpc/gen/wallet/v1"
)

// Activities holds the Temporal activity methods for the Transfer service.
// Each money/bank activity is a thin adapter around a gRPC client (Wallet,
// Bank or FX); MarkTerminal is the one activity that talks to the transfer
// repository in-process instead of over gRPC, since it only touches
// transfer's own tables (status/outbox), not a cross-service concern.
//
// None of these activities are invoked by TransferWorkflow yet — the actual
// orchestration (which activity runs when, compensation on failure) is filled
// in by a later phase. This phase only proves each activity method can dial
// its target and round-trip a call.
type Activities struct {
	wallet   walletv1.WalletServiceClient
	bank     bankv1.BankServiceClient
	fx       transferinterfaces.FXService
	transfer transferinterfaces.TransferRepository
}

// NewActivities builds the activity struct passed to worker.RegisterActivity.
// fx and transfer may be nil in tests that only exercise the gRPC-backed
// activities.
func NewActivities(
	wallet walletv1.WalletServiceClient,
	bank bankv1.BankServiceClient,
	fx transferinterfaces.FXService,
	transfer transferinterfaces.TransferRepository,
) *Activities {
	return &Activities{wallet: wallet, bank: bank, fx: fx, transfer: transfer}
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

// WalletMove calls the Wallet gRPC service's Move RPC.
func (a *Activities) WalletMove(ctx context.Context, input WalletMoveInput) (WalletMoveResult, error) {
	resp, err := a.wallet.Move(ctx, &walletv1.MoveRequest{
		TransferId:          input.TransferID.String(),
		Operation:           input.Operation,
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
		return WalletMoveResult{}, err
	}
	fromBalance, err := decimal.NewFromString(resp.GetFromAvailableBalance())
	if err != nil {
		return WalletMoveResult{}, err
	}
	toBalance, err := decimal.NewFromString(resp.GetToAvailableBalance())
	if err != nil {
		return WalletMoveResult{}, err
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
		return WalletHoldResult{}, err
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
		return WalletHoldResult{}, err
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
		return WalletHoldResult{}, err
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

// BankSubmit calls the Bank gRPC service's Submit RPC.
func (a *Activities) BankSubmit(ctx context.Context, input BankSubmitInput) (BankResult, error) {
	resp, err := a.bank.Submit(ctx, &bankv1.SubmitRequest{
		TransferId: input.TransferID.String(),
		Amount:     input.Amount.String(),
		Currency:   input.Currency,
	})
	if err != nil {
		return BankResult{}, err
	}
	return BankResult{Outcome: resp.GetOutcome(), ReferenceID: resp.GetReferenceId(), Reason: resp.GetReason()}, nil
}

// BankQuery calls the Bank gRPC service's Query RPC.
func (a *Activities) BankQuery(ctx context.Context, transferID uuid.UUID) (BankResult, error) {
	resp, err := a.bank.Query(ctx, &bankv1.QueryRequest{TransferId: transferID.String()})
	if err != nil {
		return BankResult{}, err
	}
	return BankResult{Outcome: resp.GetOutcome(), ReferenceID: resp.GetReferenceId(), Reason: resp.GetReason()}, nil
}

// MarkTerminalInput carries the terminal outcome to record on the transfer.
type MarkTerminalInput struct {
	TransferID uuid.UUID
	// Succeeded selects which repository call to make: SettleExternalTransfer
	// records this on RESERVED transfers regardless of outcome (the result
	// carries SUCCESS/FAILURE), so Succeeded is not currently branched on here
	// — it is reserved for the internal-transfer completion path a later phase
	// wires up (ExecuteInternalTransfer already advances status itself).
	Succeeded   bool
	ReferenceID string
	Reason      string
}

// MarkTerminal settles the external-transfer provider outcome on the
// transfer repository in-process (no gRPC hop): it only touches transfer's
// own tables, so it stays a direct repository call rather than a service
// boundary. It is idempotent: SettleExternalTransfer is a no-op unless the
// transfer is RESERVED.
func (a *Activities) MarkTerminal(ctx context.Context, input MarkTerminalInput) error {
	outcome := transferentities.ProviderFailure
	if input.Succeeded {
		outcome = transferentities.ProviderSuccess
	}
	return a.transfer.SettleExternalTransfer(ctx, input.TransferID, transferentities.ProviderResult{
		Outcome:     outcome,
		ReferenceID: input.ReferenceID,
		Reason:      input.Reason,
	})
}
