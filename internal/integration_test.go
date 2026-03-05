package internal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/agent"
	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/orchestrator"
	"github.com/bjk/symphony/internal/web"
	"github.com/bjk/symphony/internal/workspace"
)

// --- Test doubles ---

type stubTracker struct {
	candidates []domain.Issue
	statesByID map[string]domain.Issue
}

func (s *stubTracker) FetchCandidateIssues(_ context.Context) ([]domain.Issue, error) {
	return s.candidates, nil
}
func (s *stubTracker) FetchIssueStatesByIDs(_ context.Context, ids []string) ([]domain.Issue, error) {
	var out []domain.Issue
	for _, id := range ids {
		if issue, ok := s.statesByID[id]; ok {
			out = append(out, issue)
		}
	}
	return out, nil
}
func (s *stubTracker) FetchIssuesByStates(_ context.Context, _ []string) ([]domain.Issue, error) {
	return nil, nil
}
func (s *stubTracker) AddLabel(_ context.Context, _ int, _ string) error    { return nil }
func (s *stubTracker) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }
func (s *stubTracker) MarkPRReady(_ context.Context, _ int) error           { return nil }

type stubExecutor struct{}

func (s *stubExecutor) RunCommand(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

type stubProcessRunner struct{}
type stubProcess struct{}

func (s *stubProcess) Wait() error      { return nil }
func (s *stubProcess) Stdout() io.Reader { return bytes.NewBufferString("done") }
func (s *stubProcess) Stderr() io.Reader { return bytes.NewBufferString("") }

func (s *stubProcessRunner) Start(_ context.Context, _ string, _ []string, _ string) (agent.Process, error) {
	return &stubProcess{}, nil
}

func intPtr(v int) *int { return &v }

func integrationDeps(tr *stubTracker) (orchestrator.Deps, *config.Config) {
	cfg := &config.Config{
		Polling:   config.PollingConfig{IntervalMs: 50},
		Agent:     config.AgentConfig{MaxConcurrentAgents: 2, MaxTurns: 1, MaxRetryBackoffMs: 300000},
		Claude:    config.ClaudeConfig{StallTimeoutMs: 300000, Command: "echo"},
		Workspace: config.WorkspaceConfig{Root: "/tmp/test-int", BaseBranch: "main"},
		Tracker: config.TrackerConfig{
			TerminalStates: []string{"symphony:done", "symphony:cancelled"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := &stubExecutor{}

	deps := orchestrator.Deps{
		Tracker:   tr,
		Workspace: workspace.NewManager("/tmp/test-int", "git@github.com:o/r.git", "main", exec),
		Agent:     agent.NewRunner(cfg.Claude, &stubProcessRunner{}, logger),
		Config:    func() (*config.Config, string) { return cfg, "Work on {{ .Issue.Title }}" },
		Logger:    logger,
	}
	return deps, cfg
}

// --- Integration tests ---

func TestFullLifecycle(t *testing.T) {
	t.Parallel()

	tr := &stubTracker{
		candidates: []domain.Issue{
			{ID: "I_1", Identifier: "repo#1", Title: "Fix bug", State: "symphony:todo", Priority: intPtr(1)},
		},
		statesByID: map[string]domain.Issue{
			// Return terminal after the first tick so the agent's RunSession stops
			"I_1": {ID: "I_1", State: "symphony:done"},
		},
	}

	deps, _ := integrationDeps(tr)
	orch := orchestrator.New(deps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- orch.Run(ctx)
	}()

	// Wait for the issue to be dispatched and complete
	deadline := time.After(10 * time.Second)
	for {
		snap := orch.Snapshot()
		// Issue should pass through running and then exit (retry entry created)
		if _, hasRetry := snap.RetryAttempts["I_1"]; hasRetry {
			break
		}
		if len(snap.Running) > 0 || len(snap.RetryAttempts) > 0 {
			// In progress, keep waiting
		}
		select {
		case <-deadline:
			t.Fatalf("timed out; running=%d, retry=%d", len(snap.Running), len(snap.RetryAttempts))
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Shutdown
	orch.Events() <- domain.ShutdownEvent{}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}
}

func TestWebAPIReflectsState(t *testing.T) {
	t.Parallel()

	tr := &stubTracker{
		candidates: []domain.Issue{
			{ID: "I_1", Identifier: "repo#1", Title: "Fix", State: "symphony:todo", Priority: intPtr(1)},
		},
		statesByID: map[string]domain.Issue{
			"I_1": {ID: "I_1", State: "symphony:done"},
		},
	}

	deps, _ := integrationDeps(tr)
	orch := orchestrator.New(deps)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	webSrv := web.NewServer(orch, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- orch.Run(ctx)
	}()

	// Wait for the issue lifecycle to complete (running → exit → retry entry)
	deadline := time.After(5 * time.Second)
	for {
		snap := orch.Snapshot()
		if len(snap.Running) > 0 || len(snap.RetryAttempts) > 0 || len(snap.Claimed) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for dispatch")
		case <-time.After(25 * time.Millisecond):
		}
	}

	// Check API state returns valid JSON
	req := httptest.NewRequest("GET", "/api/v1/state", nil)
	w := httptest.NewRecorder()
	webSrv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("API state status = %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode API response: %v", err)
	}

	// Verify the response has expected structure
	if _, ok := resp["running"]; !ok {
		t.Error("missing 'running' field in API response")
	}
	if _, ok := resp["retry_queue"]; !ok {
		t.Error("missing 'retry_queue' field in API response")
	}

	// Shutdown
	orch.Events() <- domain.ShutdownEvent{}
	<-done
}

func TestConfigReloadMidRun(t *testing.T) {
	t.Parallel()

	tr := &stubTracker{}
	callCount := 0

	cfg1 := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 50},
		Agent:   config.AgentConfig{MaxConcurrentAgents: 2, MaxRetryBackoffMs: 300000},
		Claude:  config.ClaudeConfig{StallTimeoutMs: 300000},
	}
	cfg2 := &config.Config{
		Polling: config.PollingConfig{IntervalMs: 200},
		Agent:   config.AgentConfig{MaxConcurrentAgents: 8, MaxRetryBackoffMs: 300000},
		Claude:  config.ClaudeConfig{StallTimeoutMs: 600000},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := orchestrator.Deps{
		Tracker: tr,
		Config: func() (*config.Config, string) {
			callCount++
			if callCount > 1 {
				return cfg2, "new prompt"
			}
			return cfg1, "prompt"
		},
		Logger: logger,
	}

	orch := orchestrator.New(deps)

	done := make(chan error, 1)
	go func() {
		done <- orch.Run(context.Background())
	}()

	orch.Events() <- domain.WorkflowReloadEvent{}
	time.Sleep(100 * time.Millisecond)

	snap := orch.Snapshot()
	if snap.PollIntervalMs != 200 {
		t.Errorf("PollIntervalMs = %d, want 200", snap.PollIntervalMs)
	}
	if snap.MaxConcurrentAgents != 8 {
		t.Errorf("MaxConcurrentAgents = %d, want 8", snap.MaxConcurrentAgents)
	}

	orch.Events() <- domain.ShutdownEvent{}
	<-done
}

func TestRefreshEndpoint(t *testing.T) {
	t.Parallel()

	tr := &stubTracker{}
	deps, _ := integrationDeps(tr)
	orch := orchestrator.New(deps)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	webSrv := web.NewServer(orch, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- orch.Run(ctx)
	}()

	// POST /api/v1/refresh should return 202
	req := httptest.NewRequest("POST", "/api/v1/refresh", nil)
	w := httptest.NewRecorder()
	webSrv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("refresh status = %d, want %d", w.Code, http.StatusAccepted)
	}

	var resp map[string]bool
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp["queued"] {
		t.Error("expected queued=true")
	}

	orch.Events() <- domain.ShutdownEvent{}
	<-done
}
