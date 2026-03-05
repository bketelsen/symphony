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
	"github.com/bjk/symphony/internal/tracker"
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

func newCheckPRsOrchestrator(tr *mockTracker) *Orchestrator {
	cfg := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 100},
		Agent: config.AgentConfig{
			MaxConcurrentAgents: 5,
			IdleBeforeReadyMs:   60000, // 60s
		},
		Claude: config.ClaudeConfig{StallTimeoutMs: 300000},
		Tracker: config.TrackerConfig{
			TerminalStates: []string{"symphony:done", "symphony:cancelled"},
		},
	}
	return &Orchestrator{
		state: OrchestratorState{
			Running:       map[string]*domain.RunningEntry{},
			Claimed:       map[string]struct{}{},
			RetryAttempts: map[string]*domain.RetryEntry{},
			Completed:     map[string]struct{}{},
			AwaitingMerge: map[string]*domain.AwaitingMergeEntry{},
		},
		deps: Deps{
			Tracker: tr,
			Config:  func() (*config.Config, string) { return cfg, "" },
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		events: make(chan domain.Event, 64),
	}
}

func TestCheckPRsDraftIdleUndrafts(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		prStatus: &tracker.PRStatus{Found: true, Number: 5, IsDraft: true},
	}
	o := newCheckPRsOrchestrator(tr)

	// Issue claimed with retry due 2 minutes ago (well past 60s idle threshold)
	twoMinAgo := time.Now().Add(-2 * time.Minute)
	o.state.Claimed["I_1"] = struct{}{}
	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "owner/repo#42",
		Attempt:    1,
		DueAt:      twoMinAgo,
	}

	o.checkPRs(context.Background())

	// Should have called MarkPRReady
	if len(tr.markReadyCalls) != 1 || tr.markReadyCalls[0] != 5 {
		t.Errorf("markReadyCalls = %v, want [5]", tr.markReadyCalls)
	}

	// Should be moved to AwaitingMerge
	if _, ok := o.state.AwaitingMerge["I_1"]; !ok {
		t.Error("expected I_1 in AwaitingMerge")
	}

	// Should be removed from RetryAttempts and Claimed
	if _, ok := o.state.RetryAttempts["I_1"]; ok {
		t.Error("expected I_1 removed from RetryAttempts")
	}
	if _, ok := o.state.Claimed["I_1"]; ok {
		t.Error("expected I_1 removed from Claimed")
	}
}

func TestCheckPRsDraftNotIdleEnough(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		prStatus: &tracker.PRStatus{Found: true, Number: 5, IsDraft: true},
	}
	o := newCheckPRsOrchestrator(tr)

	// Retry due 10 seconds ago — within 60s idle threshold
	tenSecAgo := time.Now().Add(-10 * time.Second)
	o.state.Claimed["I_1"] = struct{}{}
	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "owner/repo#42",
		Attempt:    1,
		DueAt:      tenSecAgo,
	}

	o.checkPRs(context.Background())

	// Should NOT have called MarkPRReady
	if len(tr.markReadyCalls) != 0 {
		t.Errorf("markReadyCalls = %v, want none", tr.markReadyCalls)
	}

	// Should still be in RetryAttempts
	if _, ok := o.state.RetryAttempts["I_1"]; !ok {
		t.Error("expected I_1 to remain in RetryAttempts")
	}
}

func TestCheckPRsNoPRFound(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		prStatus: &tracker.PRStatus{Found: false},
	}
	o := newCheckPRsOrchestrator(tr)

	twoMinAgo := time.Now().Add(-2 * time.Minute)
	o.state.Claimed["I_1"] = struct{}{}
	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "owner/repo#42",
		Attempt:    1,
		DueAt:      twoMinAgo,
	}

	o.checkPRs(context.Background())

	// No undraft, no state change
	if len(tr.markReadyCalls) != 0 {
		t.Errorf("markReadyCalls = %v, want none", tr.markReadyCalls)
	}
	if _, ok := o.state.RetryAttempts["I_1"]; !ok {
		t.Error("expected I_1 to remain in RetryAttempts")
	}
}

func TestCheckPRsMergedCompletes(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		prStatus: &tracker.PRStatus{Found: true, Number: 5, Merged: true},
	}
	o := newCheckPRsOrchestrator(tr)

	o.state.AwaitingMerge["I_1"] = &domain.AwaitingMergeEntry{
		IssueID:    "I_1",
		Identifier: "owner/repo#42",
		Number:     42,
	}

	o.checkPRs(context.Background())

	// Should have added terminal label
	if len(tr.addLabelCalls) != 1 || tr.addLabelCalls[0] != "symphony:done" {
		t.Errorf("addLabelCalls = %v, want [symphony:done]", tr.addLabelCalls)
	}

	// Should have closed the issue
	if len(tr.closeCalls) != 1 || tr.closeCalls[0] != 42 {
		t.Errorf("closeCalls = %v, want [42]", tr.closeCalls)
	}

	// Should be moved to Completed
	if _, ok := o.state.Completed["I_1"]; !ok {
		t.Error("expected I_1 in Completed")
	}

	// Should be removed from AwaitingMerge
	if _, ok := o.state.AwaitingMerge["I_1"]; ok {
		t.Error("expected I_1 removed from AwaitingMerge")
	}
}

func TestCheckPRsAwaitingNotMerged(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		prStatus: &tracker.PRStatus{Found: true, Number: 5, Merged: false},
	}
	o := newCheckPRsOrchestrator(tr)

	o.state.AwaitingMerge["I_1"] = &domain.AwaitingMergeEntry{
		IssueID:    "I_1",
		Identifier: "owner/repo#42",
		Number:     42,
	}

	o.checkPRs(context.Background())

	// No completion
	if len(tr.closeCalls) != 0 {
		t.Errorf("closeCalls = %v, want none", tr.closeCalls)
	}
	if _, ok := o.state.AwaitingMerge["I_1"]; !ok {
		t.Error("expected I_1 to remain in AwaitingMerge")
	}
}

func TestCheckPRsSkipsRunningIssues(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		prStatus: &tracker.PRStatus{Found: true, Number: 5, IsDraft: true},
	}
	o := newCheckPRsOrchestrator(tr)

	twoMinAgo := time.Now().Add(-2 * time.Minute)
	o.state.Claimed["I_1"] = struct{}{}
	o.state.Running["I_1"] = &domain.RunningEntry{IssueID: "I_1"}
	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "owner/repo#42",
		Attempt:    1,
		DueAt:      twoMinAgo,
	}

	o.checkPRs(context.Background())

	// Should not undraft since agent is still running
	if len(tr.markReadyCalls) != 0 {
		t.Errorf("markReadyCalls = %v, want none (issue is running)", tr.markReadyCalls)
	}
}
