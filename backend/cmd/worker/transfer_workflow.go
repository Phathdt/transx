// Package worker holds the Transfer service's Temporal workflow and activity
// definitions. The worker process (cli.RunTransferWorker) registers these with
// an SDK worker polling temporal.task_queue.
package worker

import (
	"go.temporal.io/sdk/workflow"
)

// TransferWorkflowInput is TransferWorkflow's single argument.
type TransferWorkflowInput struct {
	TransferID string
}

// TransferWorkflow is the Transfer service's saga entry point. It is a
// skeleton: it logs receipt of the transfer id and returns without invoking
// any activity. Nothing starts this workflow yet — routing real transfer
// traffic into it (the consumer-to-Temporal bridge) and the actual
// orchestration (Wallet/Bank/FX activity calls, compensation on failure) are
// filled in by later phases. This phase only proves the worker can register
// and run a workflow definition end to end.
func TransferWorkflow(ctx workflow.Context, input TransferWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("transfer workflow received", "transferID", input.TransferID)
	return nil
}
