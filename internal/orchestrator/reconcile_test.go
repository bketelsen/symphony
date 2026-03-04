package orchestrator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
)

func TestDetectStallsStalledEntry(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.StallTimeoutMs = 300000 // 5 min

	cancelled := false
	sixMinAgo := time.Now().Add(-6 * time.Minute)
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:   "I_1",
		StartedAt: sixMinAgo,
		Cancel:    func() { cancelled = true },
	}

	o.detectStalls(time.Now())

	if !cancelled {
		t.Error("expected stalled worker to be cancelled")
	}
	if o.state.Running["I_1"].State != domain.StateStalled {
		t.Errorf("state = %q, want %q", o.state.Running["I_1"].State, domain.StateStalled)
	}
}

func TestDetectStallsNotStalled(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.StallTimeoutMs = 300000

	cancelled := false
	twoMinAgo := time.Now().Add(-2 * time.Minute)
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:   "I_1",
		StartedAt: twoMinAgo,
		Cancel:    func() { cancelled = true },
	}

	o.detectStalls(time.Now())

	if cancelled {
		t.Error("worker should not be cancelled (within timeout)")
	}
}

func TestDetectStallsUsesLastEventAt(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.StallTimeoutMs = 300000

	cancelled := false
	tenMinAgo := time.Now().Add(-10 * time.Minute)
	oneMinAgo := time.Now().Add(-1 * time.Minute)
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:     "I_1",
		StartedAt:   tenMinAgo,
		LastEventAt: &oneMinAgo, // recent activity
		Cancel:      func() { cancelled = true },
	}

	o.detectStalls(time.Now())

	if cancelled {
		t.Error("worker should not be stalled (LastEventAt is recent)")
	}
}

func TestDetectStallsDisabledWhenZero(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.StallTimeoutMs = 0

	cancelled := false
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:   "I_1",
		StartedAt: time.Now().Add(-1 * time.Hour),
		Cancel:    func() { cancelled = true },
	}

	o.detectStalls(time.Now())

	if cancelled {
		t.Error("stall detection should be disabled when timeout <= 0")
	}
}

func TestRefreshStatesTerminal(t *testing.T) {
	t.Parallel()

	cancelled := false
	tr := &mockTracker{
		statesByID: map[string]domain.Issue{
			"I_1": {ID: "I_1", State: "symphony:done"},
		},
	}

	cfg := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 100},
		Agent:   config.AgentConfig{MaxConcurrentAgents: 5},
		Claude:  config.ClaudeConfig{StallTimeoutMs: 300000},
		Tracker: config.TrackerConfig{
			TerminalStates: []string{"symphony:done", "symphony:cancelled"},
		},
	}
	o := &Orchestrator{
		state: OrchestratorState{
			Running:   map[string]*domain.RunningEntry{},
			Claimed:   map[string]struct{}{},
			RetryAttempts: map[string]*domain.RetryEntry{},
			Completed: map[string]struct{}{},
		},
		deps: Deps{
			Tracker: tr,
			Config:  func() (*config.Config, string) { return cfg, "" },
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		events: make(chan domain.Event, 64),
	}

	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		Cancel:          func() { cancelled = true },
	}

	o.refreshStates(context.Background())

	if !cancelled {
		t.Error("worker should be cancelled for terminal state")
	}
	if o.state.Running["I_1"].State != domain.StateCanceledByReconciliation {
		t.Errorf("state = %q, want %q", o.state.Running["I_1"].State, domain.StateCanceledByReconciliation)
	}
}

func TestRefreshStatesActive(t *testing.T) {
	t.Parallel()

	cancelled := false
	tr := &mockTracker{
		statesByID: map[string]domain.Issue{
			"I_1": {ID: "I_1", State: "symphony:in-progress"},
		},
	}

	cfg := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 100},
		Agent:   config.AgentConfig{MaxConcurrentAgents: 5},
		Claude:  config.ClaudeConfig{StallTimeoutMs: 300000},
		Tracker: config.TrackerConfig{
			TerminalStates: []string{"symphony:done"},
		},
	}
	o := &Orchestrator{
		state: OrchestratorState{
			Running:   map[string]*domain.RunningEntry{},
			Claimed:   map[string]struct{}{},
			RetryAttempts: map[string]*domain.RetryEntry{},
			Completed: map[string]struct{}{},
		},
		deps: Deps{
			Tracker: tr,
			Config:  func() (*config.Config, string) { return cfg, "" },
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		events: make(chan domain.Event, 64),
	}

	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID: "I_1",
		Cancel:  func() { cancelled = true },
	}

	o.refreshStates(context.Background())

	if cancelled {
		t.Error("worker should NOT be cancelled for active state")
	}
}

func TestRefreshStatesTrackerError(t *testing.T) {
	t.Parallel()

	cancelled := false
	tr := &mockTracker{err: fmt.Errorf("API down")}

	o := New(testDeps())
	o.deps.Tracker = tr
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID: "I_1",
		Cancel:  func() { cancelled = true },
	}

	o.refreshStates(context.Background())

	if cancelled {
		t.Error("worker should NOT be cancelled when tracker errors")
	}
}

func TestRefreshStatesNoRunning(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{}
	o := New(testDeps())
	o.deps.Tracker = tr

	// Should not panic with empty running map
	o.refreshStates(context.Background())
}
