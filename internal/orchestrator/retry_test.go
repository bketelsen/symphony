package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

func TestScheduleRetryContinuation(t *testing.T) {
	t.Parallel()

	o := New(testDeps())

	o.scheduleRetry("I_1", "repo#1", 1, nil, true)

	entry, ok := o.state.RetryAttempts["I_1"]
	if !ok {
		t.Fatal("retry entry not created")
	}
	if entry.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", entry.Attempt)
	}
	if !entry.IsContinuation {
		t.Error("expected IsContinuation = true")
	}

	// Continuation delay should be ~1s
	untilDue := time.Until(entry.DueAt)
	if untilDue < 500*time.Millisecond || untilDue > 2*time.Second {
		t.Errorf("DueAt should be ~1s from now, got %v", untilDue)
	}

	// Should maintain claim
	if _, claimed := o.state.Claimed["I_1"]; !claimed {
		t.Error("expected claim to be maintained")
	}

	// Verify timer fires
	select {
	case e := <-o.events:
		if rt, ok := e.(domain.RetryTimerEvent); !ok || rt.IssueID != "I_1" {
			t.Errorf("unexpected event: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timer event not received")
	}
}

func TestScheduleRetryFailure(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	errMsg := "process crashed"

	o.scheduleRetry("I_1", "repo#1", 1, &errMsg, false)

	entry := o.state.RetryAttempts["I_1"]
	if entry == nil {
		t.Fatal("retry entry not created")
	}

	// Failure attempt 1 = 10s backoff
	untilDue := time.Until(entry.DueAt)
	if untilDue < 9*time.Second || untilDue > 11*time.Second {
		t.Errorf("DueAt should be ~10s from now, got %v", untilDue)
	}

	if entry.Error == nil || *entry.Error != errMsg {
		t.Errorf("error = %v, want %q", entry.Error, errMsg)
	}
	if entry.IsContinuation {
		t.Error("expected IsContinuation = false")
	}
	// Don't wait for the 10s timer — entry validation is sufficient
}

func TestHandleWorkerExitNormal(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		StartedAt:       time.Now().Add(-10 * time.Second),
	}

	o.handleWorkerExit(domain.WorkerExitEvent{IssueID: "I_1", Err: nil})

	if _, ok := o.state.Running["I_1"]; ok {
		t.Error("should be removed from running")
	}

	entry := o.state.RetryAttempts["I_1"]
	if entry == nil {
		t.Fatal("expected continuation retry entry")
	}
	if !entry.IsContinuation {
		t.Error("expected IsContinuation = true")
	}
	if entry.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", entry.Attempt)
	}

	// AgentTotals should be updated
	if o.state.AgentTotals.SecondsRunning < 9.0 {
		t.Errorf("SecondsRunning = %f, want >= 9.0", o.state.AgentTotals.SecondsRunning)
	}
}

func TestHandleWorkerExitError(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		StartedAt:       time.Now(),
	}

	o.handleWorkerExit(domain.WorkerExitEvent{
		IssueID: "I_1",
		Err:     fmt.Errorf("agent crashed"),
	})

	entry := o.state.RetryAttempts["I_1"]
	if entry == nil {
		t.Fatal("expected retry entry")
	}
	if entry.IsContinuation {
		t.Error("expected IsContinuation = false for error")
	}
	if entry.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", entry.Attempt)
	}
	if entry.Error == nil || *entry.Error != "agent crashed" {
		t.Errorf("error = %v, want 'agent crashed'", entry.Error)
	}
}

func TestHandleWorkerExitErrorIncrementsAttempt(t *testing.T) {
	t.Parallel()

	o := New(testDeps())
	prevErr := "first failure"
	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID: "I_1",
		Attempt: 2,
		Error:   &prevErr,
	}
	o.state.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		StartedAt:       time.Now(),
	}

	o.handleWorkerExit(domain.WorkerExitEvent{
		IssueID: "I_1",
		Err:     fmt.Errorf("again"),
	})

	entry := o.state.RetryAttempts["I_1"]
	if entry.Attempt != 3 {
		t.Errorf("attempt = %d, want 3", entry.Attempt)
	}
}

func TestHandleRetryTimerEligible(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		candidates: []domain.Issue{
			{ID: "I_1", Identifier: "repo#1", Title: "Fix", State: "symphony:todo"},
		},
	}

	deps := testDispatchDeps(tr)
	o := New(deps)

	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "repo#1",
		Attempt:    1,
	}
	o.state.Claimed["I_1"] = struct{}{}

	o.handleRetryTimer(context.Background(), "I_1")

	if _, ok := o.state.Running["I_1"]; !ok {
		t.Error("expected issue to be dispatched after retry")
	}
	if _, ok := o.state.RetryAttempts["I_1"]; ok {
		t.Error("retry entry should be removed after dispatch")
	}
}

func TestHandleRetryTimerNoSlots(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		candidates: []domain.Issue{
			{ID: "I_1", Identifier: "repo#1", Title: "Fix"},
		},
	}

	deps := testDispatchDeps(tr)
	o := New(deps)
	o.state.Running["I_a"] = &domain.RunningEntry{}
	o.state.Running["I_b"] = &domain.RunningEntry{}

	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "repo#1",
		Attempt:    1,
	}
	o.state.Claimed["I_1"] = struct{}{}

	o.handleRetryTimer(context.Background(), "I_1")

	if _, ok := o.state.Running["I_1"]; ok {
		t.Error("should not dispatch when no slots")
	}
	if _, ok := o.state.RetryAttempts["I_1"]; !ok {
		t.Error("should still have retry entry (rescheduled)")
	}
	if _, ok := o.state.Claimed["I_1"]; !ok {
		t.Error("claim should be maintained")
	}
}

func TestHandleRetryTimerNotCandidate(t *testing.T) {
	t.Parallel()

	tr := &mockTracker{
		candidates: []domain.Issue{},
	}

	deps := testDispatchDeps(tr)
	o := New(deps)
	o.state.RetryAttempts["I_1"] = &domain.RetryEntry{
		IssueID:    "I_1",
		Identifier: "repo#1",
		Attempt:    1,
	}
	o.state.Claimed["I_1"] = struct{}{}

	o.handleRetryTimer(context.Background(), "I_1")

	if _, ok := o.state.Claimed["I_1"]; ok {
		t.Error("claim should be released when no longer candidate")
	}
	if _, ok := o.state.RetryAttempts["I_1"]; ok {
		t.Error("retry entry should be removed")
	}
}
