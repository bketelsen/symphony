package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/agent"
	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/workspace"
)

// mockTracker for dispatch tests
type mockTracker struct {
	candidates []domain.Issue
	statesByID map[string]domain.Issue
	err        error
}

func (m *mockTracker) FetchCandidateIssues(_ context.Context) ([]domain.Issue, error) {
	return m.candidates, m.err
}
func (m *mockTracker) FetchIssueStatesByIDs(_ context.Context, ids []string) ([]domain.Issue, error) {
	var result []domain.Issue
	for _, id := range ids {
		if issue, ok := m.statesByID[id]; ok {
			result = append(result, issue)
		}
	}
	return result, m.err
}
func (m *mockTracker) FetchIssuesByStates(_ context.Context, _ []string) ([]domain.Issue, error) {
	return nil, nil
}
func (m *mockTracker) AddLabel(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }
func (m *mockTracker) MarkPRReady(_ context.Context, _ int) error           { return nil }

// mockExecutor for workspace
type mockExecutor struct{}

func (m *mockExecutor) RunCommand(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

// mockProcessRunner for agent
type mockProcessRunner struct{}
type mockProcess struct{}

func (m *mockProcess) Wait() error      { return nil }
func (m *mockProcess) Stdout() io.Reader { return bytes.NewBufferString("done") }
func (m *mockProcess) Stderr() io.Reader { return bytes.NewBufferString("") }

func (m *mockProcessRunner) Start(_ context.Context, _ string, _ []string, _ string) (agent.Process, error) {
	return &mockProcess{}, nil
}

func intPtr(v int) *int { return &v }

func testDispatchDeps(tr *mockTracker) Deps {
	cfg := &config.Config{
		Polling:   config.PollingConfig{IntervalMs: 100},
		Agent:     config.AgentConfig{MaxConcurrentAgents: 2, MaxTurns: 1},
		Claude:    config.ClaudeConfig{StallTimeoutMs: 300000, Command: "echo"},
		Workspace: config.WorkspaceConfig{Root: "/tmp/test-ws", BaseBranch: "main"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := &mockExecutor{}

	return Deps{
		Tracker:   tr,
		Workspace: workspace.NewManager("/tmp/test-ws", "git@github.com:o/r.git", "main", exec),
		Hooks:     nil,
		Agent:     agent.NewRunner(cfg.Claude, &mockProcessRunner{}, logger),
		Config:    func() (*config.Config, string) { return cfg, "Work on {{ .Issue.Title }}" },
		Logger:    logger,
	}
}

func TestFilterCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		issues   []domain.Issue
		running  map[string]*domain.RunningEntry
		claimed  map[string]struct{}
		completed map[string]struct{}
		wantIDs  []string
	}{
		{
			name: "filters running issues",
			issues: []domain.Issue{
				{ID: "I_1", Identifier: "r#1"},
				{ID: "I_2", Identifier: "r#2"},
			},
			running: map[string]*domain.RunningEntry{"I_1": {}},
			wantIDs: []string{"I_2"},
		},
		{
			name: "filters claimed issues",
			issues: []domain.Issue{
				{ID: "I_1", Identifier: "r#1"},
				{ID: "I_2", Identifier: "r#2"},
			},
			claimed: map[string]struct{}{"I_1": {}},
			wantIDs: []string{"I_2"},
		},
		{
			name: "filters completed issues",
			issues: []domain.Issue{
				{ID: "I_1", Identifier: "r#1"},
				{ID: "I_2", Identifier: "r#2"},
			},
			completed: map[string]struct{}{"I_2": {}},
			wantIDs:   []string{"I_1"},
		},
		{
			name: "filters blocked issues",
			issues: []domain.Issue{
				{ID: "I_1", Identifier: "r#1", BlockedBy: []domain.BlockerRef{{ID: "I_3"}}},
				{ID: "I_2", Identifier: "r#2"},
			},
			wantIDs: []string{"I_2"},
		},
		{
			name: "all eligible",
			issues: []domain.Issue{
				{ID: "I_1", Identifier: "r#1"},
				{ID: "I_2", Identifier: "r#2"},
			},
			wantIDs: []string{"I_1", "I_2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := New(testDeps())
			if tt.running != nil {
				o.state.Running = tt.running
			}
			if tt.claimed != nil {
				o.state.Claimed = tt.claimed
			}
			if tt.completed != nil {
				o.state.Completed = tt.completed
			}

			got := o.filterCandidates(tt.issues)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d candidates, want %d", len(got), len(tt.wantIDs))
			}
			for i, wantID := range tt.wantIDs {
				if got[i].ID != wantID {
					t.Errorf("candidate[%d].ID = %q, want %q", i, got[i].ID, wantID)
				}
			}
		})
	}
}

func TestDispatchRespectsSlots(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		candidates: []domain.Issue{
			{ID: "I_1", Identifier: "r#1", Title: "A", Priority: intPtr(1)},
			{ID: "I_2", Identifier: "r#2", Title: "B", Priority: intPtr(2)},
			{ID: "I_3", Identifier: "r#3", Title: "C", Priority: intPtr(3)},
		},
	}

	deps := testDispatchDeps(tr)
	o := New(deps)
	// Max concurrent is 2, 1 already running
	o.state.Running["I_existing"] = &domain.RunningEntry{}

	o.dispatch(context.Background())

	// Should launch 1 worker (2 max - 1 running = 1 slot)
	if len(o.state.Running) != 2 { // existing + 1 new
		t.Errorf("running count = %d, want 2", len(o.state.Running))
	}
	// Should have launched the highest priority one (I_1)
	if _, ok := o.state.Running["I_1"]; !ok {
		t.Error("expected I_1 to be launched (highest priority)")
	}
}

func TestDispatchTrackerError(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{err: fmt.Errorf("API error")}

	deps := testDispatchDeps(tr)
	o := New(deps)
	o.state.Running["I_existing"] = &domain.RunningEntry{}

	// Should not panic or crash
	o.dispatch(context.Background())

	// Existing workers should be unaffected
	if len(o.state.Running) != 1 {
		t.Errorf("running count = %d, want 1 (unchanged)", len(o.state.Running))
	}
}

func TestDispatchNoSlots(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		candidates: []domain.Issue{{ID: "I_1", Identifier: "r#1", Title: "A"}},
	}

	deps := testDispatchDeps(tr)
	o := New(deps)
	// Fill all slots
	o.state.Running["I_a"] = &domain.RunningEntry{}
	o.state.Running["I_b"] = &domain.RunningEntry{}

	o.dispatch(context.Background())

	// No new workers should be launched
	if len(o.state.Running) != 2 {
		t.Errorf("running count = %d, want 2 (no new launches)", len(o.state.Running))
	}
}

func TestLaunchWorkerSendsExitEvent(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		// Return terminal state immediately so session ends after 1 turn
		statesByID: map[string]domain.Issue{
			"I_1": {ID: "I_1", State: "symphony:done"},
		},
	}

	deps := testDispatchDeps(tr)
	o := New(deps)

	issue := domain.Issue{
		ID:         "I_1",
		Identifier: "r#1",
		Title:      "Test",
		State:      "symphony:todo",
	}

	o.launchWorker(context.Background(), issue)

	// Wait for the worker exit event
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-o.events:
			if exit, ok := event.(domain.WorkerExitEvent); ok {
				if exit.IssueID != "I_1" {
					t.Errorf("exit event IssueID = %q, want %q", exit.IssueID, "I_1")
				}
				return
			}
			// Might get AgentUpdateEvent first, keep draining
		case <-deadline:
			t.Fatal("timed out waiting for WorkerExitEvent")
		}
	}
}
