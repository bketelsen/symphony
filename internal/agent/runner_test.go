package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
)

// mockProcess implements Process for testing.
type mockProcess struct {
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	err    error
}

func (m *mockProcess) Wait() error      { return m.err }
func (m *mockProcess) Stdout() io.Reader { return m.stdout }
func (m *mockProcess) Stderr() io.Reader { return m.stderr }

// mockProcessRunner implements ProcessRunner for testing.
type mockProcessRunner struct {
	proc  *mockProcess
	calls []processCall
}

type processCall struct {
	Cmd  string
	Args []string
	Dir  string
}

func (m *mockProcessRunner) Start(_ context.Context, cmd string, args []string, dir string) (Process, error) {
	m.calls = append(m.calls, processCall{Cmd: cmd, Args: args, Dir: dir})
	return m.proc, nil
}

// mockTracker implements tracker.TrackerClient for testing.
type mockTracker struct {
	states []domain.Issue
	err    error
	calls  int
}

func (m *mockTracker) FetchCandidateIssues(_ context.Context) ([]domain.Issue, error) {
	return nil, nil
}

func (m *mockTracker) FetchIssueStatesByIDs(_ context.Context, _ []string) ([]domain.Issue, error) {
	m.calls++
	if m.calls <= len(m.states) {
		return []domain.Issue{m.states[m.calls-1]}, m.err
	}
	return m.states[len(m.states)-1:], m.err
}

func (m *mockTracker) FetchIssuesByStates(_ context.Context, _ []string) ([]domain.Issue, error) {
	return nil, nil
}
func (m *mockTracker) AddLabel(_ context.Context, _ int, _ string) error    { return nil }
func (m *mockTracker) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }
func (m *mockTracker) MarkPRReady(_ context.Context, _ int) error           { return nil }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunTurnCommandAssembly(t *testing.T) {
	t.Parallel()

	proc := &mockProcess{
		stdout: bytes.NewBufferString("agent output"),
		stderr: bytes.NewBufferString(""),
	}
	pr := &mockProcessRunner{proc: proc}

	runner := NewRunner(config.ClaudeConfig{
		Command:        "claude --print",
		Model:          "sonnet",
		MaxTokens:      8000,
		PermissionMode: "auto",
		AllowedTools:   []string{"bash", "read"},
	}, pr, testLogger())

	output, err := runner.RunTurn(context.Background(), "do something", "/tmp/ws")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if output != "agent output" {
		t.Errorf("output = %q, want %q", output, "agent output")
	}

	if len(pr.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(pr.calls))
	}
	call := pr.calls[0]
	if call.Cmd != "claude" {
		t.Errorf("cmd = %q, want %q", call.Cmd, "claude")
	}
	if call.Dir != "/tmp/ws" {
		t.Errorf("dir = %q, want %q", call.Dir, "/tmp/ws")
	}

	args := strings.Join(call.Args, " ")
	if !strings.Contains(args, "--print") {
		t.Errorf("missing --print in args: %v", call.Args)
	}
	if !strings.Contains(args, "--model sonnet") {
		t.Errorf("missing --model sonnet in args: %v", call.Args)
	}
	if !strings.Contains(args, "do something") {
		t.Errorf("missing prompt in args: %v", call.Args)
	}
}

func TestRunTurnProcessError(t *testing.T) {
	t.Parallel()

	proc := &mockProcess{
		stdout: bytes.NewBufferString("partial output"),
		stderr: bytes.NewBufferString("error details"),
		err:    fmt.Errorf("exit status 1"),
	}
	pr := &mockProcessRunner{proc: proc}

	runner := NewRunner(config.ClaudeConfig{
		Command: "claude --print",
	}, pr, testLogger())

	output, err := runner.RunTurn(context.Background(), "do something", "/tmp")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should still capture partial stdout
	if output != "partial output" {
		t.Errorf("output = %q, want %q", output, "partial output")
	}
}

func TestRunTurnContextCancellation(t *testing.T) {
	t.Parallel()

	proc := &mockProcess{
		stdout: bytes.NewBufferString(""),
		stderr: bytes.NewBufferString(""),
		err:    context.Canceled,
	}
	pr := &mockProcessRunner{proc: proc}

	runner := NewRunner(config.ClaudeConfig{
		Command: "claude --print",
	}, pr, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := runner.RunTurn(ctx, "do something", "/tmp")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRunSessionMultiTurn(t *testing.T) {
	t.Parallel()

	turnCount := 0
	pr := &mockProcessRunner{}

	// Each turn returns a new process
	origStart := pr.Start
	_ = origStart

	// Use a custom runner that tracks turns
	proc := &mockProcess{
		stdout: bytes.NewBufferString("done"),
		stderr: bytes.NewBufferString(""),
	}
	pr.proc = proc

	// Mock tracker: active, active, then terminal state
	mt := &mockTracker{
		states: []domain.Issue{
			{ID: "I_1", State: "symphony:todo"},
			{ID: "I_1", State: "symphony:todo"},
			{ID: "I_1", State: "symphony:done"},
		},
	}

	runner := NewRunner(config.ClaudeConfig{
		Command: "claude --print",
	}, pr, testLogger())

	updates := make(chan domain.Event, 10)

	err := runner.RunSession(context.Background(), SessionParams{
		Issue:          domain.Issue{ID: "I_1", Identifier: "repo#1", Title: "Fix", State: "symphony:todo"},
		PromptTemplate: "Work on {{ .Issue.Title }}",
		MaxTurns:       5,
		Tracker:        mt,
		Updates:        updates,
		WorkDir:        "/tmp/ws",
	})
	if err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	// Should have run 3 turns (state changed on 3rd check)
	if len(pr.calls) != 3 {
		t.Errorf("got %d turns, want 3", len(pr.calls))
	}
	_ = turnCount

	// Should have 3 update events
	close(updates)
	var eventCount int
	for range updates {
		eventCount++
	}
	if eventCount != 3 {
		t.Errorf("got %d update events, want 3", eventCount)
	}
}

func TestRunSessionMaxTurns(t *testing.T) {
	t.Parallel()

	proc := &mockProcess{
		stdout: bytes.NewBufferString("output"),
		stderr: bytes.NewBufferString(""),
	}
	pr := &mockProcessRunner{proc: proc}

	// Tracker always returns active
	mt := &mockTracker{
		states: []domain.Issue{{ID: "I_1", State: "symphony:todo"}},
	}

	runner := NewRunner(config.ClaudeConfig{
		Command: "claude --print",
	}, pr, testLogger())

	err := runner.RunSession(context.Background(), SessionParams{
		Issue:          domain.Issue{ID: "I_1", State: "symphony:todo", Title: "Fix"},
		PromptTemplate: "Work on {{ .Issue.Title }}",
		MaxTurns:       2,
		Tracker:        mt,
		WorkDir:        "/tmp/ws",
	})
	if err != nil {
		t.Fatalf("RunSession: %v", err)
	}

	if len(pr.calls) != 2 {
		t.Errorf("got %d turns, want 2 (max_turns limit)", len(pr.calls))
	}
}

func TestRunSessionTurnError(t *testing.T) {
	t.Parallel()

	proc := &mockProcess{
		stdout: bytes.NewBufferString(""),
		stderr: bytes.NewBufferString(""),
		err:    fmt.Errorf("crash"),
	}
	pr := &mockProcessRunner{proc: proc}

	runner := NewRunner(config.ClaudeConfig{
		Command: "claude --print",
	}, pr, testLogger())

	err := runner.RunSession(context.Background(), SessionParams{
		Issue:          domain.Issue{ID: "I_1", State: "symphony:todo", Title: "Fix"},
		PromptTemplate: "Work on {{ .Issue.Title }}",
		MaxTurns:       5,
		WorkDir:        "/tmp/ws",
	})
	if err == nil {
		t.Fatal("expected error from turn failure")
	}

	// Should have stopped after first turn
	if len(pr.calls) != 1 {
		t.Errorf("got %d turns, want 1 (should stop on error)", len(pr.calls))
	}
}

func TestBuildArgs(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		command:        "claude --print",
		model:          "opus",
		maxTokens:      16000,
		permissionMode: "auto",
		allowedTools:   []string{"bash"},
	}

	args := runner.buildArgs("hello world")

	expected := []string{"--print", "hello world", "--model", "opus", "--max-tokens", "16000", "--permission-mode", "auto", "--allowedTools", "bash"}
	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(expected), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}
}
