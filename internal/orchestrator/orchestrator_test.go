package orchestrator

import (
	"context"
	"log/slog"
	"io"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
)

// noopTracker returns empty results for all calls.
type noopTracker struct{}

func (noopTracker) FetchCandidateIssues(_ context.Context) ([]domain.Issue, error) { return nil, nil }
func (noopTracker) FetchIssueStatesByIDs(_ context.Context, _ []string) ([]domain.Issue, error) {
	return nil, nil
}
func (noopTracker) FetchIssuesByStates(_ context.Context, _ []string) ([]domain.Issue, error) {
	return nil, nil
}

func testDeps() Deps {
	cfg := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 100},
		Agent:   config.AgentConfig{MaxConcurrentAgents: 5, MaxRetryBackoffMs: 300000},
		Claude:  config.ClaudeConfig{StallTimeoutMs: 300000},
	}
	return Deps{
		Tracker: noopTracker{},
		Config:  func() (*config.Config, string) { return cfg, "prompt" },
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestShutdownEvent(t *testing.T) {
	t.Parallel()

	o := New(testDeps())

	done := make(chan error, 1)
	go func() {
		done <- o.Run(context.Background())
	}()

	o.Events() <- domain.ShutdownEvent{}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after ShutdownEvent")
	}
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	o := New(testDeps())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestWorkerExitRemovesRunning(t *testing.T) {
	t.Parallel()

	o := New(testDeps())

	// Add a running entry
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		StartedAt:       time.Now(),
	}
	o.state.Claimed["I_1"] = struct{}{}

	done := make(chan error, 1)
	go func() {
		done <- o.Run(context.Background())
	}()

	o.Events() <- domain.WorkerExitEvent{IssueID: "I_1"}
	// Give event loop time to process
	time.Sleep(50 * time.Millisecond)
	o.Events() <- domain.ShutdownEvent{}

	<-done

	snap := o.Snapshot()
	if _, ok := snap.Running["I_1"]; ok {
		t.Error("running entry should be removed after WorkerExitEvent")
	}
	// Claimed is maintained for retry scheduling (continuation retry)
	if _, ok := snap.Claimed["I_1"]; !ok {
		t.Error("claimed entry should be maintained for retry")
	}
}

func TestAgentUpdateUpdatesRunning(t *testing.T) {
	t.Parallel()

	o := New(testDeps())

	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:   "I_1",
		TurnCount: 0,
		StartedAt: time.Now(),
	}

	done := make(chan error, 1)
	go func() {
		done <- o.Run(context.Background())
	}()

	o.Events() <- domain.AgentUpdateEvent{
		IssueID:   "I_1",
		TurnCount: 3,
		LastEvent: "turn_completed",
	}
	time.Sleep(50 * time.Millisecond)
	o.Events() <- domain.ShutdownEvent{}

	<-done

	snap := o.Snapshot()
	entry, ok := snap.Running["I_1"]
	if !ok {
		t.Fatal("running entry should still exist")
	}
	if entry.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", entry.TurnCount)
	}
	if entry.LastEvent == nil || *entry.LastEvent != "turn_completed" {
		t.Errorf("LastEvent = %v, want 'turn_completed'", entry.LastEvent)
	}
}

func TestWorkflowReload(t *testing.T) {
	t.Parallel()

	callCount := 0
	cfg := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 100},
		Agent:   config.AgentConfig{MaxConcurrentAgents: 5},
		Claude:  config.ClaudeConfig{StallTimeoutMs: 300000},
	}

	deps := Deps{
		Config: func() (*config.Config, string) {
			callCount++
			if callCount > 1 {
				// Return updated config on reload
				return &config.Config{
					Polling: config.PollingConfig{IntervalMs: 200},
					Agent:   config.AgentConfig{MaxConcurrentAgents: 10},
					Claude:  config.ClaudeConfig{StallTimeoutMs: 600000},
				}, "new prompt"
			}
			return cfg, "prompt"
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	o := New(deps)

	done := make(chan error, 1)
	go func() {
		done <- o.Run(context.Background())
	}()

	o.Events() <- domain.WorkflowReloadEvent{}
	time.Sleep(50 * time.Millisecond)
	o.Events() <- domain.ShutdownEvent{}

	<-done

	snap := o.Snapshot()
	if snap.PollIntervalMs != 200 {
		t.Errorf("PollIntervalMs = %d, want 200", snap.PollIntervalMs)
	}
	if snap.MaxConcurrentAgents != 10 {
		t.Errorf("MaxConcurrentAgents = %d, want 10", snap.MaxConcurrentAgents)
	}
}

func TestSnapshotDeepCopy(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:   "I_1",
		TurnCount: 1,
		StartedAt: time.Now(),
	}
	o.state.Claimed["I_1"] = struct{}{}

	snap := o.Snapshot()

	// Mutate the snapshot
	snap.Running["I_1"].TurnCount = 99
	delete(snap.Claimed, "I_1")

	// Original should be unchanged
	if o.state.Running["I_1"].TurnCount != 1 {
		t.Error("snapshot mutation affected original Running")
	}
	if _, ok := o.state.Claimed["I_1"]; !ok {
		t.Error("snapshot mutation affected original Claimed")
	}
}

func TestAvailableSlots(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	// Max is 5 from testDeps

	if got := o.AvailableSlots(); got != 5 {
		t.Errorf("AvailableSlots() = %d, want 5", got)
	}

	o.state.Running["I_1"] = &domain.RunningEntry{}
	o.state.Running["I_2"] = &domain.RunningEntry{}

	if got := o.AvailableSlots(); got != 3 {
		t.Errorf("AvailableSlots() = %d, want 3", got)
	}
}

func TestRefreshRequestEvent(t *testing.T) {
	t.Parallel()

	o := New(testDeps())

	done := make(chan error, 1)
	go func() {
		done <- o.Run(context.Background())
	}()

	replyCh := make(chan struct{})
	o.Events() <- domain.RefreshRequestEvent{ReplyCh: replyCh}

	select {
	case <-replyCh:
		// Reply channel was closed, meaning the refresh was processed
	case <-time.After(3 * time.Second):
		t.Fatal("RefreshRequestEvent reply not received")
	}

	o.Events() <- domain.ShutdownEvent{}
	<-done
}
