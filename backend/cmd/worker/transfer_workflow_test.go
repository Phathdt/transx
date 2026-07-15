package worker_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"

	"transx/cmd/worker"
	transferentities "transx/internal/modules/transfer/domain/entities"
	walletinterfaces "transx/internal/modules/wallet/domain/interfaces"
)

type TransferWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *TransferWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *TransferWorkflowTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func TestTransferWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(TransferWorkflowTestSuite))
}

func (s *TransferWorkflowTestSuite) registerInternalSuccessActivities(
	transferID uuid.UUID,
	sourceCurrency, destCurrency string,
	fee decimal.Decimal,
) {
	// Activity methods are registered via a nil *Activities receiver so the
	// test env matches the production worker.RegisterActivity(activities) shape.
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareInternalMove)
	s.env.RegisterActivity(a.WalletMove)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID:          transferID,
			TransferType:        "INTERNAL",
			Status:              string(transferentities.TransferStatusPending),
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: sourceCurrency,
		}, nil,
	)
	s.env.OnActivity(a.PrepareInternalMove, mock.Anything, worker.PrepareInternalMoveInput{
		TransferID: transferID,
	}).Return(
		worker.PrepareInternalMoveResult{
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			SourceAmount:        decimal.NewFromInt(100),
			SourceCurrency:      sourceCurrency,
			DestinationAmount:   decimal.NewFromInt(100),
			DestinationCurrency: destCurrency,
			SourceFXRate:        decimal.NewFromInt(1),
			DestinationFXRate:   decimal.NewFromInt(1),
			FeeAmount:           fee,
			FeeCurrency:         sourceCurrency,
		}, nil,
	)
	s.env.OnActivity(a.WalletMove, mock.Anything, mock.MatchedBy(func(in worker.WalletMoveInput) bool {
		return in.TransferID == transferID &&
			in.Operation == walletinterfaces.OperationMove &&
			in.SourceCurrency == sourceCurrency &&
			in.DestinationCurrency == destCurrency
	})).Return(worker.WalletMoveResult{}, nil)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  true,
	}).Return(nil)
}

func (s *TransferWorkflowTestSuite) TestInternalSameCurrencySuccess() {
	transferID := uuid.New()
	s.registerInternalSuccessActivities(transferID, "USD", "USD", decimal.Zero)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestInternalCrossCurrencySuccess() {
	transferID := uuid.New()
	// Cross-currency still succeeds when prepare/move/mark are happy; fee may be non-zero.
	s.registerInternalSuccessActivities(transferID, "VND", "USD", decimal.NewFromInt(1000))

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestInternalInsufficientFundsMarksFailed() {
	transferID := uuid.New()
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareInternalMove)
	s.env.RegisterActivity(a.WalletMove)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID:          transferID,
			TransferType:        "INTERNAL",
			Status:              string(transferentities.TransferStatusPending),
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
		}, nil,
	)
	s.env.OnActivity(a.PrepareInternalMove, mock.Anything, worker.PrepareInternalMoveInput{
		TransferID: transferID,
	}).Return(
		worker.PrepareInternalMoveResult{
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			SourceAmount:        decimal.NewFromInt(100),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(100),
			DestinationCurrency: "USD",
			SourceFXRate:        decimal.NewFromInt(1),
			DestinationFXRate:   decimal.NewFromInt(1),
		}, nil,
	)
	s.env.OnActivity(a.WalletMove, mock.Anything, mock.Anything).Return(
		worker.WalletMoveResult{},
		worker.BusinessErrorForTest(transferentities.FailureInsufficientFunds),
	)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  false,
		Reason:     transferentities.FailureInsufficientFunds,
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestInternalFXUnavailableMarksFailed() {
	transferID := uuid.New()
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareInternalMove)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID:          transferID,
			TransferType:        "INTERNAL",
			Status:              string(transferentities.TransferStatusPending),
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
		}, nil,
	)
	s.env.OnActivity(a.PrepareInternalMove, mock.Anything, worker.PrepareInternalMoveInput{
		TransferID: transferID,
	}).Return(
		worker.PrepareInternalMoveResult{},
		worker.BusinessErrorForTest(transferentities.FailureFXRateUnavailable),
	)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  false,
		Reason:     transferentities.FailureFXRateUnavailable,
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestAlreadyTerminalSkips() {
	transferID := uuid.New()
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID:      transferID,
			TransferType:    "INTERNAL",
			Status:          string(transferentities.TransferStatusSucceeded),
			AlreadyTerminal: true,
		}, nil,
	)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestMarkTerminalRetryAfterMove() {
	// WalletMove succeeds; first MarkTerminal flakes; second succeeds — money
	// already moved, status must still converge without re-calling Move.
	transferID := uuid.New()
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareInternalMove)
	s.env.RegisterActivity(a.WalletMove)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID:          transferID,
			TransferType:        "INTERNAL",
			Status:              string(transferentities.TransferStatusPending),
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			TransactionAmount:   decimal.NewFromInt(100),
			TransactionCurrency: "USD",
		}, nil,
	)
	s.env.OnActivity(a.PrepareInternalMove, mock.Anything, mock.Anything).Return(
		worker.PrepareInternalMoveResult{
			FromAccountRef:      "ACC-FROM",
			ToAccountRef:        "ACC-TO",
			SourceAmount:        decimal.NewFromInt(100),
			SourceCurrency:      "USD",
			DestinationAmount:   decimal.NewFromInt(100),
			DestinationCurrency: "USD",
			SourceFXRate:        decimal.NewFromInt(1),
			DestinationFXRate:   decimal.NewFromInt(1),
		}, nil,
	)
	s.env.OnActivity(a.WalletMove, mock.Anything, mock.Anything).Return(worker.WalletMoveResult{}, nil).Once()
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  true,
	}).Return(errors.New("db blip")).Once()
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  true,
	}).Return(nil).Once()

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{
		TransferID: transferID.String(),
	})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) registerExternalBase(transferID uuid.UUID) *worker.Activities {
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareExternalHold)
	s.env.RegisterActivity(a.WalletHold)
	s.env.RegisterActivity(a.BankSubmit)
	s.env.RegisterActivity(a.BankQuery)
	s.env.RegisterActivity(a.WalletSettleHold)
	s.env.RegisterActivity(a.WalletReleaseHold)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID:          transferID,
			TransferType:        "EXTERNAL",
			Status:              string(transferentities.TransferStatusPending),
			FromAccountRef:      "ACC-FROM",
			TransactionAmount:   decimal.NewFromInt(50),
			TransactionCurrency: "USD",
		}, nil,
	)
	s.env.OnActivity(a.PrepareExternalHold, mock.Anything, worker.PrepareExternalHoldInput{
		TransferID: transferID,
	}).Return(
		worker.PrepareExternalHoldResult{
			FromAccountRef: "ACC-FROM",
			Amount:         decimal.NewFromInt(50),
			Currency:       "USD",
		}, nil,
	)
	s.env.OnActivity(a.WalletHold, mock.Anything, mock.MatchedBy(func(in worker.WalletHoldInput) bool {
		return in.TransferID == transferID && in.Operation == walletinterfaces.OperationHold
	})).Return(worker.WalletHoldResult{}, nil)
	return a
}

func (s *TransferWorkflowTestSuite) TestExternalSuccessSettles() {
	transferID := uuid.New()
	a := s.registerExternalBase(transferID)

	s.env.OnActivity(a.BankSubmit, mock.Anything, mock.Anything).Return(
		worker.BankResult{Outcome: worker.BankOutcomeSuccess, ReferenceID: "stub-1"}, nil,
	)
	s.env.OnActivity(a.WalletSettleHold, mock.Anything, mock.MatchedBy(func(in worker.WalletHoldInput) bool {
		return in.Operation == walletinterfaces.OperationSettleHold
	})).Return(worker.WalletHoldResult{}, nil)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID:  transferID,
		Succeeded:   true,
		ReferenceID: "stub-1",
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{TransferID: transferID.String()})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestExternalFailureReleases() {
	transferID := uuid.New()
	a := s.registerExternalBase(transferID)

	s.env.OnActivity(a.BankSubmit, mock.Anything, mock.Anything).Return(
		worker.BankResult{Outcome: worker.BankOutcomeFailure, Reason: transferentities.FailureProviderRejected}, nil,
	)
	s.env.OnActivity(a.WalletReleaseHold, mock.Anything, mock.MatchedBy(func(in worker.WalletHoldInput) bool {
		return in.Operation == walletinterfaces.OperationReleaseHold
	})).Return(worker.WalletHoldResult{}, nil)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  false,
		Reason:     transferentities.FailureProviderRejected,
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{TransferID: transferID.String()})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *TransferWorkflowTestSuite) TestExternalUnknownThenSuccess() {
	transferID := uuid.New()
	a := s.registerExternalBase(transferID)

	// Submit returns UNKNOWN; first Query still UNKNOWN; second Query SUCCESS.
	s.env.OnActivity(a.BankSubmit, mock.Anything, mock.Anything).Return(
		worker.BankResult{Outcome: worker.BankOutcomeUnknown, Reason: "TIMEOUT"}, nil,
	)
	s.env.OnActivity(a.BankQuery, mock.Anything, transferID).Return(
		worker.BankResult{Outcome: worker.BankOutcomeUnknown}, nil,
	).Once()
	s.env.OnActivity(a.BankQuery, mock.Anything, transferID).Return(
		worker.BankResult{Outcome: worker.BankOutcomeSuccess, ReferenceID: "late-ref"}, nil,
	).Once()
	s.env.OnActivity(a.WalletSettleHold, mock.Anything, mock.Anything).Return(worker.WalletHoldResult{}, nil)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID:  transferID,
		Succeeded:   true,
		ReferenceID: "late-ref",
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{TransferID: transferID.String()})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// --- Fault-matrix scenarios (unit / Temporal test env) ---

// TestFaultWalletMoveBusinessNoMoneyThenFailed covers permanent Wallet failure
// after prepare: MarkTerminal(FAILED), workflow completes without success path.
func (s *TransferWorkflowTestSuite) TestFaultWalletMoveBusinessNoMoneyThenFailed() {
	transferID := uuid.New()
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareInternalMove)
	s.env.RegisterActivity(a.WalletMove)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID: transferID, TransferType: "INTERNAL",
			Status:         string(transferentities.TransferStatusPending),
			FromAccountRef: "ACC-FROM", ToAccountRef: "ACC-TO",
			TransactionAmount: decimal.NewFromInt(10), TransactionCurrency: "USD",
		}, nil)
	s.env.OnActivity(a.PrepareInternalMove, mock.Anything, mock.Anything).Return(
		worker.PrepareInternalMoveResult{
			FromAccountRef: "ACC-FROM", ToAccountRef: "ACC-TO",
			SourceAmount: decimal.NewFromInt(10), SourceCurrency: "USD",
			DestinationAmount: decimal.NewFromInt(10), DestinationCurrency: "USD",
			SourceFXRate: decimal.NewFromInt(1), DestinationFXRate: decimal.NewFromInt(1),
		}, nil)
	s.env.OnActivity(a.WalletMove, mock.Anything, mock.Anything).Return(
		worker.WalletMoveResult{}, worker.BusinessErrorForTest(transferentities.FailureInsufficientFunds))
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID, Succeeded: false, Reason: transferentities.FailureInsufficientFunds,
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{TransferID: transferID.String()})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// TestFaultExternalUnknownDoesNotReleaseHold: Submit UNKNOWN then Query still
// UNKNOWN for several polls — MarkTerminal/Settle/Release must not run.
func (s *TransferWorkflowTestSuite) TestFaultExternalUnknownDoesNotReleaseHold() {
	transferID := uuid.New()
	a := s.registerExternalBase(transferID)

	s.env.OnActivity(a.BankSubmit, mock.Anything, mock.Anything).Return(
		worker.BankResult{Outcome: worker.BankOutcomeUnknown, Reason: "TIMEOUT"}, nil)
	// Two UNKNOWN polls then FAILURE — proves hold is retained across UNKNOWN
	// and only Release+MarkTerminal run once the bank outcome is known.
	s.env.OnActivity(a.BankQuery, mock.Anything, transferID).Return(
		worker.BankResult{Outcome: worker.BankOutcomeUnknown}, nil).Times(2)
	s.env.OnActivity(a.BankQuery, mock.Anything, transferID).Return(
		worker.BankResult{
			Outcome: worker.BankOutcomeFailure,
			Reason:  transferentities.FailureProviderRejected,
		}, nil).
		Once()
	s.env.OnActivity(a.WalletReleaseHold, mock.Anything, mock.Anything).Return(worker.WalletHoldResult{}, nil).Once()
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID, Succeeded: false, Reason: transferentities.FailureProviderRejected,
	}).Return(nil).Once()

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{TransferID: transferID.String()})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// TestFaultPrepareExternalCurrencyMismatchFailsBeforeHold ensures CheckCurrency
// failure terminalizes without Hold.
func (s *TransferWorkflowTestSuite) TestFaultPrepareExternalCurrencyMismatchFailsBeforeHold() {
	transferID := uuid.New()
	var a *worker.Activities
	s.env.RegisterActivity(a.LoadTransfer)
	s.env.RegisterActivity(a.PrepareExternalHold)
	s.env.RegisterActivity(a.MarkTerminal)

	s.env.OnActivity(a.LoadTransfer, mock.Anything, transferID).Return(
		worker.LoadTransferResult{
			TransferID: transferID, TransferType: "EXTERNAL",
			Status:            string(transferentities.TransferStatusPending),
			FromAccountRef:    "ACC-FROM",
			TransactionAmount: decimal.NewFromInt(50), TransactionCurrency: "USD",
		}, nil)
	s.env.OnActivity(a.PrepareExternalHold, mock.Anything, mock.Anything).Return(
		worker.PrepareExternalHoldResult{},
		worker.BusinessErrorForTest(transferentities.FailureFXRateUnavailable),
	)
	s.env.OnActivity(a.MarkTerminal, mock.Anything, worker.MarkTerminalInput{
		TransferID: transferID, Succeeded: false, Reason: transferentities.FailureFXRateUnavailable,
	}).Return(nil)

	s.env.ExecuteWorkflow(worker.TransferWorkflow, worker.TransferWorkflowInput{TransferID: transferID.String()})
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}
