package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

// reconcile checks for stalled workers, refreshes issue states, and checks PR status.
func (o *Orchestrator) reconcile(ctx context.Context) {
	o.detectStalls(time.Now())
	o.refreshStates(ctx)
	o.checkPRs(ctx)
}

// detectStalls checks each running entry against stall_timeout_ms.
// If a worker hasn't reported activity within the timeout, it is cancelled.
func (o *Orchestrator) detectStalls(now time.Time) {
	if o.state.StallTimeoutMs <= 0 {
		return
	}

	timeout := time.Duration(o.state.StallTimeoutMs) * time.Millisecond

	for id, entry := range o.state.Running {
		lastActivity := entry.StartedAt
		if entry.LastEventAt != nil {
			lastActivity = *entry.LastEventAt
		}

		if now.Sub(lastActivity) > timeout {
			o.logEvent("stall_detected", id, entry.IssueIdentifier, "no activity for "+now.Sub(lastActivity).Round(time.Second).String())
			o.deps.Logger.Warn("stall detected, cancelling worker",
				"issue_id", id,
				"issue_identifier", entry.IssueIdentifier,
				"last_activity", lastActivity,
				"stall_timeout_ms", o.state.StallTimeoutMs,
			)
			entry.State = domain.StateStalled
			if entry.Cancel != nil {
				entry.Cancel()
			}
		}
	}
}

// refreshStates fetches current issue states and handles transitions.
// If an issue has moved to a terminal state, the worker is cancelled.
func (o *Orchestrator) refreshStates(ctx context.Context) {
	if len(o.state.Running) == 0 {
		return
	}

	ids := make([]string, 0, len(o.state.Running))
	for id := range o.state.Running {
		ids = append(ids, id)
	}

	issues, err := o.deps.Tracker.FetchIssueStatesByIDs(ctx, ids)
	if err != nil {
		o.deps.Logger.Error("reconcile: failed to fetch issue states", "error", err)
		return
	}

	cfg, _ := o.deps.Config()
	terminalSet := make(map[string]struct{}, len(cfg.Tracker.TerminalStates))
	for _, s := range cfg.Tracker.TerminalStates {
		terminalSet[s] = struct{}{}
	}

	for _, issue := range issues {
		entry, ok := o.state.Running[issue.ID]
		if !ok {
			continue
		}

		if _, terminal := terminalSet[issue.State]; terminal {
			o.deps.Logger.Info("issue reached terminal state, cancelling worker",
				"issue_id", issue.ID,
				"issue_identifier", issue.Identifier,
				"state", issue.State,
			)

			// Mark PR ready for review
			go func(num int, id string) {
				if err := o.deps.Tracker.MarkPRReady(context.Background(), num); err != nil {
					o.deps.Logger.Warn("failed to mark PR ready", "issue_id", id, "issue_number", num, "error", err)
				}
			}(issue.Number, issue.ID)

			entry.State = domain.StateCanceledByReconciliation
			if entry.Cancel != nil {
				entry.Cancel()
			}
		}
	}
}

// checkPRs handles two responsibilities:
// 1. For claimed (not running) issues with draft PRs that have been idle long enough: undraft and move to AwaitingMerge.
// 2. For AwaitingMerge issues: detect merged PRs and mark issues done.
func (o *Orchestrator) checkPRs(ctx context.Context) {
	cfg, _ := o.deps.Config()
	idleThreshold := time.Duration(cfg.Agent.IdleBeforeReadyMs) * time.Millisecond
	now := time.Now()

	// 1. Check claimed issues that are not running — look for idle agents with draft PRs to undraft.
	for issueID, retryEntry := range o.state.RetryAttempts {
		if _, running := o.state.Running[issueID]; running {
			continue
		}
		if _, awaiting := o.state.AwaitingMerge[issueID]; awaiting {
			continue
		}

		// Check if idle long enough past the retry DueAt
		if now.Sub(retryEntry.DueAt) < idleThreshold {
			continue
		}

		// Check PR status
		status, err := o.deps.Tracker.GetPRStatus(ctx, issueNumberFromRetryEntry(retryEntry))
		if err != nil {
			o.deps.Logger.Warn("checkPRs: failed to get PR status", "issue_id", issueID, "error", err)
			continue
		}

		if !status.Found || !status.IsDraft {
			continue
		}

		// Undraft the PR
		if err := o.deps.Tracker.MarkPRReady(ctx, status.Number); err != nil {
			o.deps.Logger.Warn("checkPRs: failed to mark PR ready", "issue_id", issueID, "error", err)
			continue
		}

		o.logEvent("pr_ready", issueID, retryEntry.Identifier,
			fmt.Sprintf("PR #%d marked ready for review (agent idle)", status.Number))

		o.deps.Logger.Info("PR undrafted, moving to awaiting merge",
			"issue_id", issueID,
			"pr_number", status.Number,
		)

		// Move to AwaitingMerge
		o.state.AwaitingMerge[issueID] = &domain.AwaitingMergeEntry{
			IssueID:    issueID,
			Identifier: retryEntry.Identifier,
			Number:     issueNumberFromRetryEntry(retryEntry),
		}
		delete(o.state.RetryAttempts, issueID)
		delete(o.state.Claimed, issueID)
	}

	// 2. Check AwaitingMerge issues for merged PRs.
	for issueID, entry := range o.state.AwaitingMerge {
		status, err := o.deps.Tracker.GetPRStatus(ctx, entry.Number)
		if err != nil {
			o.deps.Logger.Warn("checkPRs: failed to get PR status for awaiting merge", "issue_id", issueID, "error", err)
			continue
		}

		if !status.Found || !status.Merged {
			continue
		}

		// PR merged — add terminal label, close issue, move to Completed
		if len(cfg.Tracker.TerminalStates) > 0 {
			terminalLabel := cfg.Tracker.TerminalStates[0]
			if err := o.deps.Tracker.AddLabel(ctx, entry.Number, terminalLabel); err != nil {
				o.deps.Logger.Warn("checkPRs: failed to add terminal label", "issue_id", issueID, "error", err)
			}
		}

		if err := o.deps.Tracker.CloseIssue(ctx, entry.Number); err != nil {
			o.deps.Logger.Warn("checkPRs: failed to close issue", "issue_id", issueID, "error", err)
		}

		o.logEvent("issue_completed", issueID, entry.Identifier,
			fmt.Sprintf("PR #%d merged, issue closed", status.Number))

		o.deps.Logger.Info("PR merged, issue completed",
			"issue_id", issueID,
			"pr_number", status.Number,
		)

		o.state.Completed[issueID] = struct{}{}
		delete(o.state.AwaitingMerge, issueID)
	}
}

// issueNumberFromRetryEntry extracts the issue number from a retry entry's identifier.
func issueNumberFromRetryEntry(entry *domain.RetryEntry) int {
	// Identifier format: "owner/repo#123"
	for i := len(entry.Identifier) - 1; i >= 0; i-- {
		if entry.Identifier[i] == '#' {
			n := 0
			for _, c := range entry.Identifier[i+1:] {
				n = n*10 + int(c-'0')
			}
			return n
		}
	}
	return 0
}
