package orchestrator

import (
	"context"
	"time"

	"github.com/bjk/symphony/internal/agent"
	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/workspace"
)

// dispatch fetches candidates, filters, sorts, and launches workers up to available slots.
func (o *Orchestrator) dispatch(ctx context.Context) {
	slots := o.availableSlots()
	if slots <= 0 {
		return
	}

	issues, err := o.deps.Tracker.FetchCandidateIssues(ctx)
	if err != nil {
		o.deps.Logger.Error("dispatch: failed to fetch candidates", "error", err)
		return
	}

	candidates := o.filterCandidates(issues)
	domain.SortCandidates(candidates)

	if len(candidates) > slots {
		candidates = candidates[:slots]
	}

	for _, issue := range candidates {
		o.launchWorker(ctx, issue)
	}
}

// filterCandidates removes issues that are already running, claimed, completed, or blocked.
func (o *Orchestrator) filterCandidates(issues []domain.Issue) []domain.Issue {
	var result []domain.Issue
	for _, issue := range issues {
		if _, running := o.state.Running[issue.ID]; running {
			continue
		}
		if _, claimed := o.state.Claimed[issue.ID]; claimed {
			continue
		}
		if _, completed := o.state.Completed[issue.ID]; completed {
			continue
		}
		if issue.IsBlocked(o.state.Completed) {
			continue
		}
		result = append(result, issue)
	}
	return result
}

// launchWorker starts a goroutine for the given issue.
func (o *Orchestrator) launchWorker(ctx context.Context, issue domain.Issue) {
	sessionID := issue.ID + "-" + time.Now().Format("20060102-150405")
	workerCtx, cancel := context.WithCancel(ctx)

	o.state.Running[issue.ID] = &domain.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           domain.StatePreparingWorkspace,
		SessionID:       sessionID,
		StartedAt:       time.Now(),
		Cancel:          cancel,
	}
	o.state.Claimed[issue.ID] = struct{}{}

	// Swap label: first active state (todo) → second active state (in-progress)
	cfg, promptTemplate := o.deps.Config()
	if len(cfg.Tracker.ActiveStates) >= 2 {
		todoLabel := cfg.Tracker.ActiveStates[0]
		inProgressLabel := cfg.Tracker.ActiveStates[1]
		go func() {
			bgCtx := context.Background()
			if err := o.deps.Tracker.RemoveLabel(bgCtx, issue.Number, todoLabel); err != nil {
				o.deps.Logger.Warn("failed to remove label", "issue_id", issue.ID, "label", todoLabel, "error", err)
			}
			if err := o.deps.Tracker.AddLabel(bgCtx, issue.Number, inProgressLabel); err != nil {
				o.deps.Logger.Warn("failed to add label", "issue_id", issue.ID, "label", inProgressLabel, "error", err)
			}
		}()
	}

	key := workspace.SanitizeKey(issue.Identifier)

	go func() {
		err := o.runWorker(workerCtx, issue, key, sessionID, promptTemplate, cfg)
		o.events <- domain.WorkerExitEvent{
			IssueID: issue.ID,
			Err:     err,
		}
	}()

	o.logEvent("dispatched", issue.ID, issue.Identifier, "worker launched, session "+sessionID)

	o.deps.Logger.Info("worker launched",
		"issue_id", issue.ID,
		"issue_identifier", issue.Identifier,
		"session_id", sessionID,
	)
}

// runWorker executes the full worker lifecycle: workspace setup, hooks, agent session.
func (o *Orchestrator) runWorker(
	ctx context.Context,
	issue domain.Issue,
	key, sessionID, promptTemplate string,
	cfg *config.Config,
) error {
	// Create workspace
	wsPath, created, err := o.deps.Workspace.Create(ctx, key)
	if err != nil {
		return err
	}

	// Run after_create hook if workspace was just created
	if created && o.deps.Hooks != nil {
		if _, err := o.deps.Hooks.RunHook(ctx, "after_create", wsPath); err != nil {
			return err
		}
	}

	// Run before_run hook
	if o.deps.Hooks != nil {
		if _, err := o.deps.Hooks.RunHook(ctx, "before_run", wsPath); err != nil {
			return err
		}
	}

	// Determine attempt number from retry state
	attempt := 0
	if entry, ok := o.state.RetryAttempts[issue.ID]; ok {
		attempt = entry.Attempt
	}

	// Run agent session
	err = o.deps.Agent.RunSession(ctx, agent.SessionParams{
		Issue:          issue,
		PromptTemplate: promptTemplate,
		Attempt:        attempt,
		WorkDir:        wsPath,
		MaxTurns:       cfg.Agent.MaxTurns,
		Tracker:        o.deps.Tracker,
		Updates:        o.events,
		SessionID:      sessionID,
	})

	// Run after_run hook (non-fatal)
	if o.deps.Hooks != nil {
		if _, hookErr := o.deps.Hooks.RunHook(ctx, "after_run", wsPath); hookErr != nil {
			o.deps.Logger.Warn("after_run hook failed", "error", hookErr)
		}
	}

	return err
}
