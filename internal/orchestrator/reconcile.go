package orchestrator

import (
	"context"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

// reconcile checks for stalled workers and refreshes issue states from tracker.
func (o *Orchestrator) reconcile(ctx context.Context) {
	o.detectStalls(time.Now())
	o.refreshStates(ctx)
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
