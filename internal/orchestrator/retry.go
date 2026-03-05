package orchestrator

import (
	"context"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

// handleWorkerExit processes a WorkerExitEvent.
// Normal exit (nil error) → continuation retry (1s delay).
// Error exit → exponential backoff retry.
func (o *Orchestrator) handleWorkerExit(e domain.WorkerExitEvent) {
	entry, wasRunning := o.state.Running[e.IssueID]
	delete(o.state.Running, e.IssueID)

	// Update agent totals if we had a running entry
	if wasRunning {
		elapsed := time.Since(entry.StartedAt).Seconds()
		o.state.AgentTotals.SecondsRunning += elapsed
		o.state.AgentTotals.InputTokens += entry.InputTokens
		o.state.AgentTotals.OutputTokens += entry.OutputTokens
		o.state.AgentTotals.TotalTokens += entry.TotalTokens
	}

	identifier := e.IssueID
	if wasRunning {
		identifier = entry.IssueIdentifier
	}

	if e.Err == nil {
		// Normal exit → continuation retry
		o.deps.Logger.Info("worker completed normally, scheduling continuation",
			"issue_id", e.IssueID,
		)
		o.scheduleRetry(e.IssueID, identifier, 1, nil, true)
	} else {
		// Error exit → exponential backoff
		prevAttempt := 0
		if prev, ok := o.state.RetryAttempts[e.IssueID]; ok {
			prevAttempt = prev.Attempt
		}
		errMsg := e.Err.Error()
		o.deps.Logger.Warn("worker failed, scheduling retry",
			"issue_id", e.IssueID,
			"error", errMsg,
			"attempt", prevAttempt+1,
		)
		o.scheduleRetry(e.IssueID, identifier, prevAttempt+1, &errMsg, false)
	}
}

// scheduleRetry adds an issue to the retry queue and starts a timer goroutine.
func (o *Orchestrator) scheduleRetry(issueID, identifier string, attempt int, errMsg *string, isContinuation bool) {
	cfg, _ := o.deps.Config()
	delay := domain.CalculateBackoff(attempt, isContinuation, cfg.Agent.MaxRetryBackoffMs)
	dueAt := time.Now().Add(delay)

	o.state.RetryAttempts[issueID] = &domain.RetryEntry{
		IssueID:        issueID,
		Identifier:     identifier,
		Attempt:        attempt,
		DueAt:          dueAt,
		Error:          errMsg,
		IsContinuation: isContinuation,
	}

	// Keep the claim so dispatch doesn't pick it up
	o.state.Claimed[issueID] = struct{}{}

	o.deps.Logger.Info("retry scheduled",
		"issue_id", issueID,
		"attempt", attempt,
		"delay", delay,
		"due_at", dueAt,
		"is_continuation", isContinuation,
	)

	// Start timer goroutine
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
		o.events <- domain.RetryTimerEvent{IssueID: issueID}
	}()
}

// handleRetryTimer processes a RetryTimerEvent.
// Checks if the issue is still eligible, then dispatches or releases the claim.
func (o *Orchestrator) handleRetryTimer(ctx context.Context, issueID string) {
	retryEntry, ok := o.state.RetryAttempts[issueID]
	if !ok {
		return
	}

	// Check if we have slots
	if o.availableSlots() <= 0 {
		errMsg := "no available orchestrator slots"
		o.deps.Logger.Info("retry deferred, no slots",
			"issue_id", issueID,
		)
		// Reschedule with same attempt number
		o.scheduleRetry(issueID, retryEntry.Identifier, retryEntry.Attempt, &errMsg, retryEntry.IsContinuation)
		return
	}

	// Fetch fresh candidate list to check eligibility
	candidates, err := o.deps.Tracker.FetchCandidateIssues(ctx)
	if err != nil {
		o.deps.Logger.Error("retry: failed to check candidates", "error", err)
		// Reschedule
		errMsg := "failed to check eligibility: " + err.Error()
		o.scheduleRetry(issueID, retryEntry.Identifier, retryEntry.Attempt, &errMsg, retryEntry.IsContinuation)
		return
	}

	// Find the issue in candidates
	var found *domain.Issue
	for _, c := range candidates {
		if c.ID == issueID {
			found = &c
			break
		}
	}

	if found == nil {
		// Issue no longer a candidate, release claim
		o.deps.Logger.Info("retry: issue no longer candidate, releasing claim",
			"issue_id", issueID,
		)
		delete(o.state.RetryAttempts, issueID)
		delete(o.state.Claimed, issueID)
		return
	}

	// Dispatch the retry
	delete(o.state.RetryAttempts, issueID)
	delete(o.state.Claimed, issueID)
	o.launchWorker(ctx, *found)
}
