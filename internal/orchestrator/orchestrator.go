package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/bjk/symphony/internal/agent"
	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/tracker"
	"github.com/bjk/symphony/internal/workspace"
)

// Deps holds external dependencies for the orchestrator.
type Deps struct {
	Tracker   tracker.TrackerClient
	Workspace *workspace.Manager
	Hooks     *workspace.HookRunner
	Agent     *agent.Runner
	Config    func() (*config.Config, string) // returns current config + prompt
	Logger    *slog.Logger
}

// OrchestratorState holds all mutable orchestrator state.
type OrchestratorState struct {
	PollIntervalMs      int
	MaxConcurrentAgents int
	StallTimeoutMs      int
	Running             map[string]*domain.RunningEntry
	Claimed             map[string]struct{}
	RetryAttempts       map[string]*domain.RetryEntry
	Completed           map[string]struct{}
	AwaitingMerge       map[string]*domain.AwaitingMergeEntry
	AgentTotals         domain.AgentTotals
	EventLog            []domain.EventLogEntry
	StartedAt           time.Time
}

// Orchestrator runs the main event loop.
type Orchestrator struct {
	mu     sync.RWMutex // protects state for Snapshot reads
	state  OrchestratorState
	deps   Deps
	events chan domain.Event
	done   chan struct{}
}

// New creates an orchestrator with the given dependencies.
func New(deps Deps) *Orchestrator {
	cfg, _ := deps.Config()

	return &Orchestrator{
		state: OrchestratorState{
			PollIntervalMs:      cfg.Polling.IntervalMs,
			MaxConcurrentAgents: cfg.Agent.MaxConcurrentAgents,
			StallTimeoutMs:      cfg.Claude.StallTimeoutMs,
			Running:             make(map[string]*domain.RunningEntry),
			Claimed:             make(map[string]struct{}),
			RetryAttempts:       make(map[string]*domain.RetryEntry),
			Completed:           make(map[string]struct{}),
			AwaitingMerge:       make(map[string]*domain.AwaitingMergeEntry),
			StartedAt:           time.Now(),
		},
		deps:   deps,
		events: make(chan domain.Event, 64),
		done:   make(chan struct{}),
	}
}

// Events returns the event channel for external producers.
func (o *Orchestrator) Events() chan<- domain.Event {
	return o.events
}

// Snapshot returns a deep copy of the current state (safe for concurrent access).
func (o *Orchestrator) Snapshot() OrchestratorState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return OrchestratorState{
		PollIntervalMs:      o.state.PollIntervalMs,
		MaxConcurrentAgents: o.state.MaxConcurrentAgents,
		StallTimeoutMs:      o.state.StallTimeoutMs,
		Running:             copyRunningMap(o.state.Running),
		Claimed:             maps.Clone(o.state.Claimed),
		RetryAttempts:       copyRetryMap(o.state.RetryAttempts),
		Completed:           maps.Clone(o.state.Completed),
		AwaitingMerge:       copyAwaitingMergeMap(o.state.AwaitingMerge),
		AgentTotals:         o.state.AgentTotals,
		EventLog:            copyEventLog(o.state.EventLog),
		StartedAt:           o.state.StartedAt,
	}
}

// AvailableSlots returns how many more workers can be started.
// When called from the event loop (under mu.Lock), use availableSlots() instead.
func (o *Orchestrator) AvailableSlots() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.availableSlots()
}

func (o *Orchestrator) availableSlots() int {
	slots := o.state.MaxConcurrentAgents - len(o.state.Running)
	if slots < 0 {
		return 0
	}
	return slots
}

// Run starts the event loop. Blocks until ShutdownEvent or context cancellation.
func (o *Orchestrator) Run(ctx context.Context) error {
	defer close(o.done)

	ticker := time.NewTicker(time.Duration(o.state.PollIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	o.deps.Logger.Info("orchestrator started",
		"poll_interval_ms", o.state.PollIntervalMs,
		"max_concurrent", o.state.MaxConcurrentAgents,
	)

	for {
		select {
		case <-ctx.Done():
			o.deps.Logger.Info("orchestrator shutting down (context cancelled)")
			return ctx.Err()

		case <-ticker.C:
			o.events <- domain.TickEvent{}

		case event, ok := <-o.events:
			if !ok {
				return nil
			}

			o.mu.Lock()
			switch e := event.(type) {
			case domain.ShutdownEvent:
				o.mu.Unlock()
				o.deps.Logger.Info("orchestrator shutting down (shutdown event)")
				return nil

			case domain.TickEvent:
				o.handleTick(ctx)

			case domain.WorkerExitEvent:
				o.handleWorkerExit(e)

			case domain.AgentUpdateEvent:
				o.handleAgentUpdate(e)

			case domain.RetryTimerEvent:
				o.handleRetryTimer(ctx, e.IssueID)

			case domain.WorkflowReloadEvent:
				o.handleWorkflowReload(ticker)

			case domain.RefreshRequestEvent:
				o.handleTick(ctx)
				if e.ReplyCh != nil {
					close(e.ReplyCh)
				}
			}
			o.mu.Unlock()
		}
	}
}

func (o *Orchestrator) handleTick(ctx context.Context) {
	o.dispatch(ctx)
	o.reconcile(ctx)
}

func (o *Orchestrator) handleAgentUpdate(e domain.AgentUpdateEvent) {
	entry, ok := o.state.Running[e.IssueID]
	if !ok {
		return
	}
	now := time.Now()
	entry.State = domain.StateStreamingTurn
	entry.TurnCount = e.TurnCount
	entry.LastEventAt = &now
	lastEvent := e.LastEvent
	entry.LastEvent = &lastEvent
	lastMessage := e.LastMessage
	entry.LastMessage = &lastMessage
	entry.InputTokens = e.InputTokens
	entry.OutputTokens = e.OutputTokens

	o.logEvent("turn_completed", e.IssueID, entry.IssueIdentifier,
		fmt.Sprintf("turn %d: %s", e.TurnCount, truncateStr(e.LastMessage, 80)))

	o.deps.Logger.Info("agent update",
		"issue_id", e.IssueID,
		"turn", e.TurnCount,
		"event", e.LastEvent,
	)
}

func (o *Orchestrator) handleWorkflowReload(ticker *time.Ticker) {
	cfg, _ := o.deps.Config()
	o.deps.Logger.Info("config reloaded",
		"poll_interval_ms", cfg.Polling.IntervalMs,
		"max_concurrent", cfg.Agent.MaxConcurrentAgents,
	)
	o.state.PollIntervalMs = cfg.Polling.IntervalMs
	o.state.MaxConcurrentAgents = cfg.Agent.MaxConcurrentAgents
	o.state.StallTimeoutMs = cfg.Claude.StallTimeoutMs
	ticker.Reset(time.Duration(cfg.Polling.IntervalMs) * time.Millisecond)
}

// Deep copy helpers

func copyRunningMap(m map[string]*domain.RunningEntry) map[string]*domain.RunningEntry {
	out := make(map[string]*domain.RunningEntry, len(m))
	for k, v := range m {
		cp := *v
		cp.Cancel = nil // don't expose cancel func in snapshots
		out[k] = &cp
	}
	return out
}

func copyRetryMap(m map[string]*domain.RetryEntry) map[string]*domain.RetryEntry {
	out := make(map[string]*domain.RetryEntry, len(m))
	for k, v := range m {
		cp := *v
		out[k] = &cp
	}
	return out
}

func copyAwaitingMergeMap(m map[string]*domain.AwaitingMergeEntry) map[string]*domain.AwaitingMergeEntry {
	out := make(map[string]*domain.AwaitingMergeEntry, len(m))
	for k, v := range m {
		cp := *v
		out[k] = &cp
	}
	return out
}

func copyEventLog(log []domain.EventLogEntry) []domain.EventLogEntry {
	if log == nil {
		return nil
	}
	out := make([]domain.EventLogEntry, len(log))
	copy(out, log)
	return out
}

const maxEventLogSize = 100

// logEvent appends an event to the event log, capping at maxEventLogSize.
func (o *Orchestrator) logEvent(kind, issueID, identifier, message string) {
	entry := domain.EventLogEntry{
		Timestamp:  time.Now(),
		IssueID:    issueID,
		Identifier: identifier,
		Kind:       kind,
		Message:    message,
	}
	o.state.EventLog = append(o.state.EventLog, entry)
	if len(o.state.EventLog) > maxEventLogSize {
		o.state.EventLog = o.state.EventLog[len(o.state.EventLog)-maxEventLogSize:]
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
