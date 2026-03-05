package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

// stateResponse is the JSON shape for GET /api/v1/state.
type stateResponse struct {
	PollIntervalMs      int              `json:"poll_interval_ms"`
	MaxConcurrentAgents int              `json:"max_concurrent_agents"`
	Running             []runningJSON    `json:"running"`
	RetryQueue          []retryJSON      `json:"retry_queue"`
	CompletedCount      int              `json:"completed_count"`
	AgentTotals         agentTotalsJSON  `json:"agent_totals"`
	EventLog            []eventJSON      `json:"event_log"`
	Uptime              string           `json:"uptime"`
}

type eventJSON struct {
	Timestamp  string `json:"timestamp"`
	IssueID    string `json:"issue_id"`
	Identifier string `json:"identifier"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
}

type runningJSON struct {
	IssueID         string  `json:"issue_id"`
	IssueIdentifier string  `json:"issue_identifier"`
	State           string  `json:"state"`
	SessionID       string  `json:"session_id"`
	TurnCount       int     `json:"turn_count"`
	StartedAt       string  `json:"started_at"`
	LastEvent       *string `json:"last_event,omitempty"`
	LastMessage     *string `json:"last_message,omitempty"`
}

type retryJSON struct {
	IssueID        string  `json:"issue_id"`
	Identifier     string  `json:"identifier"`
	Attempt        int     `json:"attempt"`
	DueAt          string  `json:"due_at"`
	Error          *string `json:"error,omitempty"`
	IsContinuation bool    `json:"is_continuation"`
}

type agentTotalsJSON struct {
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	TotalTokens    int     `json:"total_tokens"`
	SecondsRunning float64 `json:"seconds_running"`
}

func (s *Server) handleAPIState(w http.ResponseWriter, _ *http.Request) {
	snap := s.state.Snapshot()

	running := make([]runningJSON, 0, len(snap.Running))
	for _, entry := range snap.Running {
		running = append(running, runningJSON{
			IssueID:         entry.IssueID,
			IssueIdentifier: entry.IssueIdentifier,
			State:           string(entry.State),
			SessionID:       entry.SessionID,
			TurnCount:       entry.TurnCount,
			StartedAt:       entry.StartedAt.Format(time.RFC3339),
			LastEvent:       entry.LastEvent,
			LastMessage:     entry.LastMessage,
		})
	}

	retryQueue := make([]retryJSON, 0, len(snap.RetryAttempts))
	for _, entry := range snap.RetryAttempts {
		retryQueue = append(retryQueue, retryJSON{
			IssueID:        entry.IssueID,
			Identifier:     entry.Identifier,
			Attempt:        entry.Attempt,
			DueAt:          entry.DueAt.Format(time.RFC3339),
			Error:          entry.Error,
			IsContinuation: entry.IsContinuation,
		})
	}

	events := make([]eventJSON, 0, len(snap.EventLog))
	for i := len(snap.EventLog) - 1; i >= 0; i-- {
		e := snap.EventLog[i]
		events = append(events, eventJSON{
			Timestamp:  e.Timestamp.Format(time.RFC3339),
			IssueID:    e.IssueID,
			Identifier: e.Identifier,
			Kind:       e.Kind,
			Message:    e.Message,
		})
	}

	resp := stateResponse{
		PollIntervalMs:      snap.PollIntervalMs,
		MaxConcurrentAgents: snap.MaxConcurrentAgents,
		Running:             running,
		RetryQueue:          retryQueue,
		CompletedCount:      len(snap.Completed),
		AgentTotals: agentTotalsJSON{
			InputTokens:    snap.AgentTotals.InputTokens,
			OutputTokens:   snap.AgentTotals.OutputTokens,
			TotalTokens:    snap.AgentTotals.TotalTokens,
			SecondsRunning: snap.AgentTotals.SecondsRunning,
		},
		EventLog: events,
		Uptime:   time.Since(snap.StartedAt).Round(time.Second).String(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAPIIssue(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("identifier")
	snap := s.state.Snapshot()

	// Check running
	for _, entry := range snap.Running {
		if entry.IssueIdentifier == identifier || entry.IssueID == identifier {
			writeJSON(w, http.StatusOK, runningJSON{
				IssueID:         entry.IssueID,
				IssueIdentifier: entry.IssueIdentifier,
				State:           string(entry.State),
				SessionID:       entry.SessionID,
				TurnCount:       entry.TurnCount,
				StartedAt:       entry.StartedAt.Format(time.RFC3339),
				LastEvent:       entry.LastEvent,
				LastMessage:     entry.LastMessage,
			})
			return
		}
	}

	// Check retry queue
	for _, entry := range snap.RetryAttempts {
		if entry.Identifier == identifier || entry.IssueID == identifier {
			writeJSON(w, http.StatusOK, retryJSON{
				IssueID:        entry.IssueID,
				Identifier:     entry.Identifier,
				Attempt:        entry.Attempt,
				DueAt:          entry.DueAt.Format(time.RFC3339),
				Error:          entry.Error,
				IsContinuation: entry.IsContinuation,
			})
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "issue not found",
	})
}

func (s *Server) handleAPIRefresh(w http.ResponseWriter, _ *http.Request) {
	replyCh := make(chan struct{})
	s.state.Events() <- domain.RefreshRequestEvent{ReplyCh: replyCh}

	// Don't block — just queue and return 202
	writeJSON(w, http.StatusAccepted, map[string]bool{
		"queued": true,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
