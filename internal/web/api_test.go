package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

func TestAPIState(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	provider.snapshot.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		State:           domain.StateStreamingTurn,
		TurnCount:       3,
		StartedAt:       time.Now().Add(-5 * time.Minute),
	}
	provider.snapshot.Completed["I_2"] = struct{}{}

	srv := testServer(provider)

	req := httptest.NewRequest("GET", "/api/v1/state", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp stateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.PollIntervalMs != 30000 {
		t.Errorf("PollIntervalMs = %d, want 30000", resp.PollIntervalMs)
	}
	if resp.MaxConcurrentAgents != 5 {
		t.Errorf("MaxConcurrentAgents = %d, want 5", resp.MaxConcurrentAgents)
	}
	if len(resp.Running) != 1 {
		t.Fatalf("running count = %d, want 1", len(resp.Running))
	}
	if resp.Running[0].IssueID != "I_1" {
		t.Errorf("running[0].IssueID = %q, want I_1", resp.Running[0].IssueID)
	}
	if resp.CompletedCount != 1 {
		t.Errorf("CompletedCount = %d, want 1", resp.CompletedCount)
	}
}

func TestAPIIssueFound(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	provider.snapshot.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		State:           domain.StateStreamingTurn,
		TurnCount:       2,
		StartedAt:       time.Now(),
	}

	srv := testServer(provider)

	req := httptest.NewRequest("GET", "/api/v1/issues/repo#1", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp runningJSON
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.IssueID != "I_1" {
		t.Errorf("IssueID = %q, want I_1", resp.IssueID)
	}
}

func TestAPIIssueNotFound(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	srv := testServer(provider)

	req := httptest.NewRequest("GET", "/api/v1/issues/unknown", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAPIRefresh(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	srv := testServer(provider)

	req := httptest.NewRequest("POST", "/api/v1/refresh", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}

	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if !resp["queued"] {
		t.Error("expected queued=true")
	}

	// Verify the event was sent to the channel
	select {
	case e := <-provider.events:
		if _, ok := e.(domain.RefreshRequestEvent); !ok {
			t.Errorf("expected RefreshRequestEvent, got %T", e)
		}
	default:
		t.Error("no event sent to events channel")
	}
}
