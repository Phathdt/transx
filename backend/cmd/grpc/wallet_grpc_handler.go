package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/modules/wallet/domain/interfaces"
	walletv1 "transx/internal/platform/grpc/gen/wallet/v1"
)

// WalletServer adapts interfaces.MoneyRepository to the generated gRPC
// WalletService. Decimal values cross the wire as strings to preserve
// precision. Every RPC is idempotent on (transfer_id, operation): the
// repository checks/writes the wallet operation guard in the same transaction
// as the money movement.
type WalletServer struct {
	walletv1.UnimplementedWalletServiceServer
	money interfaces.MoneyRepository
}

func NewWalletServer(money interfaces.MoneyRepository) *WalletServer {
	return &WalletServer{money: money}
}

func (s *WalletServer) Move(ctx context.Context, req *walletv1.MoveRequest) (*walletv1.MoveResponse, error) {
	transferID, err := uuid.Parse(req.GetTransferId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid transfer_id %q: %v", req.GetTransferId(), err)
	}
	if req.GetOperation() == "" {
		return nil, status.Error(codes.InvalidArgument, "operation is required")
	}

	sourceAmount, err := decimal.NewFromString(req.GetSourceAmount())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid source_amount %q: %v", req.GetSourceAmount(), err)
	}
	destinationAmount, err := decimal.NewFromString(req.GetDestinationAmount())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"invalid destination_amount %q: %v",
			req.GetDestinationAmount(),
			err,
		)
	}
	feeAmount := decimal.Zero
	if req.GetFeeAmount() != "" {
		feeAmount, err = decimal.NewFromString(req.GetFeeAmount())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid fee_amount %q: %v", req.GetFeeAmount(), err)
		}
	}

	result, err := s.money.Move(ctx, transferID, req.GetOperation(), interfaces.MoveInput{
		FromAccountRef:      req.GetFromAccountRef(),
		ToAccountRef:        req.GetToAccountRef(),
		SourceAmount:        sourceAmount,
		SourceCurrency:      req.GetSourceCurrency(),
		DestinationAmount:   destinationAmount,
		DestinationCurrency: req.GetDestinationCurrency(),
		FeeAmount:           feeAmount,
		FeeCurrency:         req.GetFeeCurrency(),
	})
	if err != nil {
		return nil, moneyRepositoryError(err)
	}
	return &walletv1.MoveResponse{
		FromAvailableBalance: result.FromAvailableBalance.String(),
		ToAvailableBalance:   result.ToAvailableBalance.String(),
	}, nil
}

func (s *WalletServer) Hold(ctx context.Context, req *walletv1.HoldRequest) (*walletv1.HoldResponse, error) {
	transferID, amount, err := parseTransferAndAmount(req.GetTransferId(), req.GetOperation(), req.GetAmount())
	if err != nil {
		return nil, err
	}

	result, err := s.money.Hold(ctx, transferID, req.GetOperation(), req.GetAccountRef(), amount, req.GetCurrency())
	if err != nil {
		return nil, moneyRepositoryError(err)
	}
	return &walletv1.HoldResponse{
		AvailableBalance: result.AvailableBalance.String(),
		HoldBalance:      result.HoldBalance.String(),
	}, nil
}

func (s *WalletServer) SettleHold(
	ctx context.Context,
	req *walletv1.SettleHoldRequest,
) (*walletv1.SettleHoldResponse, error) {
	transferID, amount, err := parseTransferAndAmount(req.GetTransferId(), req.GetOperation(), req.GetAmount())
	if err != nil {
		return nil, err
	}

	result, err := s.money.SettleHold(
		ctx,
		transferID,
		req.GetOperation(),
		req.GetAccountRef(),
		amount,
		req.GetCurrency(),
	)
	if err != nil {
		return nil, moneyRepositoryError(err)
	}
	return &walletv1.SettleHoldResponse{
		AvailableBalance: result.AvailableBalance.String(),
		HoldBalance:      result.HoldBalance.String(),
	}, nil
}

func (s *WalletServer) ReleaseHold(
	ctx context.Context,
	req *walletv1.ReleaseHoldRequest,
) (*walletv1.ReleaseHoldResponse, error) {
	transferID, amount, err := parseTransferAndAmount(req.GetTransferId(), req.GetOperation(), req.GetAmount())
	if err != nil {
		return nil, err
	}

	result, err := s.money.ReleaseHold(
		ctx,
		transferID,
		req.GetOperation(),
		req.GetAccountRef(),
		amount,
		req.GetCurrency(),
	)
	if err != nil {
		return nil, moneyRepositoryError(err)
	}
	return &walletv1.ReleaseHoldResponse{
		AvailableBalance: result.AvailableBalance.String(),
		HoldBalance:      result.HoldBalance.String(),
	}, nil
}

// parseTransferAndAmount validates the fields shared by Hold/SettleHold/ReleaseHold.
func parseTransferAndAmount(transferIDRaw, operation, amountRaw string) (uuid.UUID, decimal.Decimal, error) {
	transferID, err := uuid.Parse(transferIDRaw)
	if err != nil {
		return uuid.Nil, decimal.Decimal{}, status.Errorf(
			codes.InvalidArgument,
			"invalid transfer_id %q: %v",
			transferIDRaw,
			err,
		)
	}
	if operation == "" {
		return uuid.Nil, decimal.Decimal{}, status.Error(codes.InvalidArgument, "operation is required")
	}
	amount, err := decimal.NewFromString(amountRaw)
	if err != nil {
		return uuid.Nil, decimal.Decimal{}, status.Errorf(
			codes.InvalidArgument,
			"invalid amount %q: %v",
			amountRaw,
			err,
		)
	}
	return transferID, amount, nil
}

// moneyRepositoryError maps interfaces.MoneyRepository sentinel errors onto
// gRPC status codes; anything else is an internal error.
func moneyRepositoryError(err error) error {
	switch {
	case errors.Is(err, interfaces.ErrAccountNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, interfaces.ErrAccountNotActive):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, interfaces.ErrCurrencyMismatch):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, interfaces.ErrInsufficientFunds):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
