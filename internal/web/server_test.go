package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/orchestrator"
)

type mockStateProvider struct {
	snapshot orchestrator.OrchestratorState
	events   chan domain.Event
}

func (m *mockStateProvider) Snapshot() orchestrator.OrchestratorState {
	return m.snapshot
}

func (m *mockStateProvider) Events() chan<- domain.Event {
	return m.events
}

func newMockProvider() *mockStateProvider {
	return &mockStateProvider{
		snapshot: orchestrator.OrchestratorState{
			PollIntervalMs:      30000,
			MaxConcurrentAgents: 5,
			Running:             map[string]*domain.RunningEntry{},
			Claimed:             map[string]struct{}{},
			RetryAttempts:       map[string]*domain.RetryEntry{},
			Completed:           map[string]struct{}{},
			StartedAt:           time.Now(),
		},
		events: make(chan domain.Event, 10),
	}
}

func testServer(provider *mockStateProvider) *Server {
	return NewServer(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestGetDashboard(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	srv := testServer(provider)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Symphony Dashboard") {
		t.Error("missing dashboard title")
	}
	if !strings.Contains(body, "htmx") {
		t.Error("missing htmx script")
	}
	if !strings.Contains(body, "tailwindcss") {
		t.Error("missing tailwind script")
	}
	if !strings.Contains(body, "hx-get") {
		t.Error("missing HTMX attributes")
	}
}

func TestGetPartialState(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	provider.snapshot.Running["I_1"] = &domain.RunningEntry{
		IssueID:         "I_1",
		IssueIdentifier: "repo#1",
		State:           domain.StateStreamingTurn,
		TurnCount:       3,
		StartedAt:       time.Now().Add(-5 * time.Minute),
	}

	srv := testServer(provider)

	req := httptest.NewRequest("GET", "/partials/state", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	// Should be a partial (no DOCTYPE)
	if strings.Contains(body, "DOCTYPE") {
		t.Error("partial should not contain full page markup")
	}
	if !strings.Contains(body, "repo#1") {
		t.Error("missing issue identifier")
	}
	if !strings.Contains(body, "Active Sessions") {
		t.Error("missing Active Sessions heading")
	}
}

func TestGetPartialEvents(t *testing.T) {
	t.Parallel()

	provider := newMockProvider()
	srv := testServer(provider)

	req := httptest.NewRequest("GET", "/partials/events", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if strings.Contains(body, "DOCTYPE") {
		t.Error("partial should not contain full page markup")
	}
}
