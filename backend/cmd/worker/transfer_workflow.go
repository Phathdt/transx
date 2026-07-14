// Package worker holds the Transfer service's Temporal workflow and activity
// definitions. The worker process (cli.RunTransferWorker) registers these with
// an SDK worker polling temporal.task_queue.
package worker

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	transferentities "transx/internal/modules/transfer/domain/entities"
	walletinterfaces "transx/internal/modules/wallet/domain/interfaces"
)

// TransferWorkflowInput is TransferWorkflow's single argument.
type TransferWorkflowInput struct {
	TransferID string
}

const (
	transferTypeInternal = "INTERNAL"
	transferTypeExternal = "EXTERNAL"

	// activityStartToClose is the per-attempt budget for one activity call.
	activityStartToClose = 30 * time.Second
	// activityScheduleToClose bounds total time across retries for one activity
	// on the normal path (quote/hold/move/bank).
	activityScheduleToClose = 5 * time.Minute

	// terminalScheduleToClose bounds MarkTerminal retries after money has moved.
	// Money and status live in different transactions; keep retrying until they
	// converge rather than abandoning a PROCESSING transfer with committed money.
	terminalScheduleToClose = 24 * time.Hour

	// bankUnknownPollInterval is the wait between Bank.Query attempts while the
	// bank outcome is UNKNOWN/timeout. Hold stays in place for the whole poll.
	bankUnknownPollInterval = 30 * time.Second
	// bankUnknownAlertAfter is when the workflow logs a high-severity alert that
	// the hold is stuck waiting on bank reconciliation (ops, not auto-release).
	bankUnknownAlertAfter = 15 * time.Minute
	// bankUnknownMaxWait caps how long the workflow polls before returning a
	// retryable error so Temporal can continue the poll on a new run attempt
	// without terminalizing (hold remains).
	bankUnknownMaxWait = 24 * time.Hour
)

// TransferWorkflow orchestrates a transfer saga.
//
// INTERNAL: prepare (quote + freeze settlement) → Wallet.Move → MarkTerminal.
// EXTERNAL: prepare (currency check + freeze) → Wallet.Hold → Bank.Submit →
//
//	SUCCESS: SettleHold + MarkTerminal(SUCCEEDED)
//	FAILURE: ReleaseHold + MarkTerminal(FAILED)
//	UNKNOWN: keep hold, poll Bank.Query, never auto-release/settle.
func TransferWorkflow(ctx workflow.Context, input TransferWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("transfer workflow started", "transferID", input.TransferID)

	transferID, err := uuid.Parse(input.TransferID)
	if err != nil {
		return fmt.Errorf("invalid transfer id %q: %w", input.TransferID, err)
	}

	actx := workflow.WithActivityOptions(ctx, defaultActivityOptions())
	var activities *Activities

	var loaded LoadTransferResult
	if err := workflow.ExecuteActivity(actx, activities.LoadTransfer, transferID).Get(ctx, &loaded); err != nil {
		return err
	}
	if loaded.AlreadyTerminal {
		logger.Info("transfer already terminal, skipping", "transferID", input.TransferID, "status", loaded.Status)
		return nil
	}

	switch loaded.TransferType {
	case transferTypeInternal:
		return runInternalTransfer(ctx, actx, activities, transferID)
	case transferTypeExternal:
		return runExternalTransfer(ctx, actx, activities, transferID)
	default:
		logger.Error("unsupported transfer type for workflow", "type", loaded.TransferType)
		// Terminalize so a mis-routed type does not stick PENDING after the
		// consumer has already started the workflow and marked inbox processed.
		_ = workflow.ExecuteActivity(terminalActivityContext(ctx), activities.MarkTerminal, MarkTerminalInput{
			TransferID: transferID,
			Succeeded:  false,
			Reason:     transferentities.FailureProviderRejected,
		}).Get(ctx, nil)
		return temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("unsupported transfer type %q", loaded.TransferType),
			BusinessErrorType,
			nil,
		)
	}
}

func defaultActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    activityStartToClose,
		ScheduleToCloseTimeout: activityScheduleToClose,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    10,
			NonRetryableErrorTypes: []string{
				BusinessErrorType,
			},
		},
	}
}

// terminalActivityContext uses a long schedule-to-close and unlimited attempts
// so MarkTerminal keeps retrying after money has already moved.
func terminalActivityContext(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout:    activityStartToClose,
		ScheduleToCloseTimeout: terminalScheduleToClose,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    0, // unlimited until schedule-to-close
			NonRetryableErrorTypes: []string{
				BusinessErrorType,
			},
		},
	})
}

// runInternalTransfer executes prepare → Move → MarkTerminal(SUCCEEDED).
// On business failure, marks FAILED with the reason.
func runInternalTransfer(
	ctx workflow.Context,
	actx workflow.Context,
	activities *Activities,
	transferID uuid.UUID,
) error {
	logger := workflow.GetLogger(ctx)
	tctx := terminalActivityContext(ctx)

	var prepared PrepareInternalMoveResult
	if err := workflow.ExecuteActivity(actx, activities.PrepareInternalMove, PrepareInternalMoveInput{
		TransferID: transferID,
	}).Get(ctx, &prepared); err != nil {
		return markFailedIfBusiness(ctx, tctx, activities, transferID, err)
	}
	if prepared.AlreadyTerminal {
		logger.Info("transfer already terminal during prepare, skipping", "transferID", transferID.String())
		return nil
	}

	moveInput := WalletMoveInput{
		TransferID:          transferID,
		Operation:           walletinterfaces.OperationMove,
		FromAccountRef:      prepared.FromAccountRef,
		ToAccountRef:        prepared.ToAccountRef,
		SourceAmount:        prepared.SourceAmount,
		SourceCurrency:      prepared.SourceCurrency,
		DestinationAmount:   prepared.DestinationAmount,
		DestinationCurrency: prepared.DestinationCurrency,
		FeeAmount:           prepared.FeeAmount,
		FeeCurrency:         prepared.FeeCurrency,
	}
	if err := workflow.ExecuteActivity(actx, activities.WalletMove, moveInput).Get(ctx, nil); err != nil {
		return markFailedIfBusiness(ctx, tctx, activities, transferID, err)
	}

	if err := workflow.ExecuteActivity(tctx, activities.MarkTerminal, MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  true,
	}).Get(ctx, nil); err != nil {
		return err
	}

	logger.Info("internal transfer succeeded", "transferID", transferID.String())
	return nil
}

// runExternalTransfer is the EXTERNAL saga: currency check + hold → bank submit
// → settle or release. UNKNOWN keeps the hold and polls Bank.Query.
func runExternalTransfer(
	ctx workflow.Context,
	actx workflow.Context,
	activities *Activities,
	transferID uuid.UUID,
) error {
	logger := workflow.GetLogger(ctx)
	tctx := terminalActivityContext(ctx)

	var prepared PrepareExternalHoldResult
	if err := workflow.ExecuteActivity(actx, activities.PrepareExternalHold, PrepareExternalHoldInput{
		TransferID: transferID,
	}).Get(ctx, &prepared); err != nil {
		return markFailedIfBusiness(ctx, tctx, activities, transferID, err)
	}
	if prepared.AlreadyTerminal {
		logger.Info("external transfer already terminal during prepare, skipping", "transferID", transferID.String())
		return nil
	}

	holdInput := WalletHoldInput{
		TransferID: transferID,
		Operation:  walletinterfaces.OperationHold,
		AccountRef: prepared.FromAccountRef,
		Amount:     prepared.Amount,
		Currency:   prepared.Currency,
	}
	if err := workflow.ExecuteActivity(actx, activities.WalletHold, holdInput).Get(ctx, nil); err != nil {
		return markFailedIfBusiness(ctx, tctx, activities, transferID, err)
	}

	var bank BankResult
	if err := workflow.ExecuteActivity(actx, activities.BankSubmit, BankSubmitInput{
		TransferID: transferID,
		Amount:     prepared.Amount,
		Currency:   prepared.Currency,
	}).Get(ctx, &bank); err != nil {
		// Transient bank errors (non-timeout) bubble for Temporal retry of the
		// activity; the hold stays (op-guard makes Hold redelivery a no-op).
		return err
	}

	return settleExternalFromBank(ctx, actx, tctx, activities, transferID, prepared, bank)
}

// settleExternalFromBank branches on bank outcome after Submit (or a later Query).
func settleExternalFromBank(
	ctx workflow.Context,
	actx workflow.Context,
	tctx workflow.Context,
	activities *Activities,
	transferID uuid.UUID,
	prepared PrepareExternalHoldResult,
	bank BankResult,
) error {
	logger := workflow.GetLogger(ctx)

	switch bank.Outcome {
	case BankOutcomeSuccess:
		settleInput := WalletHoldInput{
			TransferID: transferID,
			Operation:  walletinterfaces.OperationSettleHold,
			AccountRef: prepared.FromAccountRef,
			Amount:     prepared.Amount,
			Currency:   prepared.Currency,
		}
		if err := workflow.ExecuteActivity(actx, activities.WalletSettleHold, settleInput).Get(ctx, nil); err != nil {
			return err
		}
		if err := workflow.ExecuteActivity(tctx, activities.MarkTerminal, MarkTerminalInput{
			TransferID:  transferID,
			Succeeded:   true,
			ReferenceID: bank.ReferenceID,
		}).Get(ctx, nil); err != nil {
			return err
		}
		logger.Info("external transfer succeeded", "transferID", transferID.String())
		return nil

	case BankOutcomeFailure:
		releaseInput := WalletHoldInput{
			TransferID: transferID,
			Operation:  walletinterfaces.OperationReleaseHold,
			AccountRef: prepared.FromAccountRef,
			Amount:     prepared.Amount,
			Currency:   prepared.Currency,
		}
		if err := workflow.ExecuteActivity(actx, activities.WalletReleaseHold, releaseInput).Get(ctx, nil); err != nil {
			return err
		}
		reason := bank.Reason
		if reason == "" {
			reason = transferentities.FailureProviderRejected
		}
		if err := workflow.ExecuteActivity(tctx, activities.MarkTerminal, MarkTerminalInput{
			TransferID: transferID,
			Succeeded:  false,
			Reason:     reason,
		}).Get(ctx, nil); err != nil {
			return err
		}
		logger.Info("external transfer failed", "transferID", transferID.String(), "reason", reason)
		return nil

	default:
		// UNKNOWN / timeout: keep hold, poll Bank.Query. Never auto-release.
		return pollBankUntilKnown(ctx, actx, tctx, activities, transferID, prepared)
	}
}

// pollBankUntilKnown waits and queries the bank until SUCCESS/FAILURE or the
// max wait elapses. Hold is never released here; after max wait the workflow
// returns a retryable error so Temporal reschedules without terminalizing.
func pollBankUntilKnown(
	ctx workflow.Context,
	actx workflow.Context,
	tctx workflow.Context,
	activities *Activities,
	transferID uuid.UUID,
	prepared PrepareExternalHoldResult,
) error {
	logger := workflow.GetLogger(ctx)
	start := workflow.Now(ctx)
	alerted := false

	for {
		elapsed := workflow.Now(ctx).Sub(start)
		if elapsed >= bankUnknownMaxWait {
			logger.Error("external transfer bank UNKNOWN exceeded max wait; hold retained for manual reconciliation",
				"transferID", transferID.String(),
				"elapsed", elapsed.String(),
			)
			// Retryable: Temporal will continue the workflow later; hold stays.
			return fmt.Errorf("bank outcome still UNKNOWN after %s for transfer %s", elapsed, transferID)
		}
		if !alerted && elapsed >= bankUnknownAlertAfter {
			logger.Error("external transfer bank UNKNOWN past alert threshold; hold retained",
				"transferID", transferID.String(),
				"elapsed", elapsed.String(),
				"threshold", bankUnknownAlertAfter.String(),
			)
			alerted = true
		}

		if err := workflow.Sleep(ctx, bankUnknownPollInterval); err != nil {
			return err
		}

		var bank BankResult
		if err := workflow.ExecuteActivity(actx, activities.BankQuery, transferID).Get(ctx, &bank); err != nil {
			// Transient query failure: continue polling (hold retained).
			logger.Warn("bank query failed during UNKNOWN poll; will retry",
				"transferID", transferID.String(),
				"error", err.Error(),
			)
			continue
		}
		if bank.Outcome == BankOutcomeSuccess || bank.Outcome == BankOutcomeFailure {
			return settleExternalFromBank(ctx, actx, tctx, activities, transferID, prepared, bank)
		}
		// Still UNKNOWN — loop.
	}
}

// markFailedIfBusiness records FAILED + reason when err is a permanent business
// failure, then returns nil so the workflow completes (transfer is terminal).
// Transient errors are returned so Temporal retries the workflow task path.
func markFailedIfBusiness(
	ctx workflow.Context,
	tctx workflow.Context,
	activities *Activities,
	transferID uuid.UUID,
	err error,
) error {
	if !isBusinessError(err) {
		return err
	}
	reason := businessReason(err)
	if markErr := workflow.ExecuteActivity(tctx, activities.MarkTerminal, MarkTerminalInput{
		TransferID: transferID,
		Succeeded:  false,
		Reason:     reason,
	}).Get(ctx, nil); markErr != nil {
		return markErr
	}
	return nil
}
