package grpc

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/modules/fx/application/services"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
)

// FXServer adapts the FX application service to the generated gRPC FXService.
// Decimal values cross the wire as strings to preserve precision.
type FXServer struct {
	fxv1.UnimplementedFXServiceServer
	svc *services.ConfigService
}

func NewFXServer(svc *services.ConfigService) *FXServer {
	return &FXServer{svc: svc}
}

func (s *FXServer) Quote(_ context.Context, req *fxv1.QuoteRequest) (*fxv1.QuoteResponse, error) {
	amount, err := decimal.NewFromString(req.GetAmount())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid amount %q: %v", req.GetAmount(), err)
	}
	quote, err := s.svc.Quote(amount, req.GetFromCurrency(), req.GetToCurrency())
	if err != nil {
		if errors.Is(err, services.ErrRateUnavailable) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &fxv1.QuoteResponse{
		Amount:   quote.Amount.String(),
		Currency: quote.Currency,
		Rate:     quote.Rate.String(),
		Source:   quote.Source,
	}, nil
}

func (s *FXServer) QuoteFee(_ context.Context, req *fxv1.QuoteFeeRequest) (*fxv1.QuoteFeeResponse, error) {
	fee := s.svc.QuoteFee(req.GetTransactionCurrency(), req.GetSourceCurrency())
	return &fxv1.QuoteFeeResponse{
		Amount:   fee.Amount.String(),
		Currency: fee.Currency,
	}, nil
}
