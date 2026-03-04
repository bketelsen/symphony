package orchestrator

import (
	"context"

	"github.com/bjk/symphony/internal/domain"
)

// handleWorkerExit processes a WorkerExitEvent.
// Full implementation in Task 12.
func (o *Orchestrator) handleWorkerExit(e domain.WorkerExitEvent) {
	// Remove from running
	delete(o.state.Running, e.IssueID)
	delete(o.state.Claimed, e.IssueID)

	// Update totals for elapsed time
	// Full retry scheduling in Task 12
}

// handleRetryTimer processes a RetryTimerEvent.
// Full implementation in Task 12.
func (o *Orchestrator) handleRetryTimer(ctx context.Context, issueID string) {
	// stub - will be implemented in Task 12
	_ = ctx
	_ = issueID
}
