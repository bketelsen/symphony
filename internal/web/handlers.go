package web

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Symphony Dashboard</title>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-900 text-gray-100 min-h-screen">
  <div class="max-w-7xl mx-auto px-4 py-6">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">Symphony Dashboard</h1>
      <span class="text-green-400 text-sm animate-pulse">&#x25CF; Live</span>
    </div>

    <div class="grid grid-cols-5 gap-4 mb-6">
      <div class="bg-gray-800 rounded-lg p-4">
        <div class="text-gray-400 text-sm">Running</div>
        <div class="text-2xl font-bold">{{.RunningCount}}/{{.MaxConcurrent}}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-4">
        <div class="text-gray-400 text-sm">Retrying</div>
        <div class="text-2xl font-bold">{{.RetryCount}}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-4">
        <div class="text-gray-400 text-sm">Completed</div>
        <div class="text-2xl font-bold">{{.CompletedCount}}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-4">
        <div class="text-gray-400 text-sm">Tokens</div>
        <div class="text-2xl font-bold">{{.TotalTokens}}</div>
      </div>
      <div class="bg-gray-800 rounded-lg p-4">
        <div class="text-gray-400 text-sm">Uptime</div>
        <div class="text-2xl font-bold">{{.Uptime}}</div>
      </div>
    </div>

    <div hx-get="/partials/state" hx-trigger="every 3s" hx-swap="innerHTML">
      {{.StatePartial}}
    </div>
  </div>
</body>
</html>`))

var statePartialTmpl = template.Must(template.New("state").Parse(`
<div class="mb-6">
  <h2 class="text-lg font-semibold mb-3">Active Sessions</h2>
  <div class="bg-gray-800 rounded-lg overflow-hidden">
    <table class="w-full text-sm">
      <thead class="bg-gray-700">
        <tr>
          <th class="px-4 py-2 text-left">Issue</th>
          <th class="px-4 py-2 text-left">State</th>
          <th class="px-4 py-2 text-left">Turns</th>
          <th class="px-4 py-2 text-left">Time</th>
          <th class="px-4 py-2 text-left">Last Message</th>
        </tr>
      </thead>
      <tbody>
        {{range .Running}}
        <tr class="border-t border-gray-700">
          <td class="px-4 py-2 font-mono">{{.IssueIdentifier}}</td>
          <td class="px-4 py-2 {{.StateColor}}">{{.State}}</td>
          <td class="px-4 py-2">{{.TurnCount}}</td>
          <td class="px-4 py-2">{{.Elapsed}}</td>
          <td class="px-4 py-2 text-gray-400 truncate max-w-md" title="{{.LastMessage}}">{{.LastMessage}}</td>
        </tr>
        {{else}}
        <tr><td colspan="5" class="px-4 py-4 text-gray-500 text-center">No active sessions</td></tr>
        {{end}}
      </tbody>
    </table>
  </div>
</div>

<div class="mb-6">
  <h2 class="text-lg font-semibold mb-3">Retry Queue</h2>
  <div class="bg-gray-800 rounded-lg overflow-hidden">
    <table class="w-full text-sm">
      <thead class="bg-gray-700">
        <tr>
          <th class="px-4 py-2 text-left">Issue</th>
          <th class="px-4 py-2 text-left">Attempt</th>
          <th class="px-4 py-2 text-left">Due In</th>
          <th class="px-4 py-2 text-left">Error</th>
        </tr>
      </thead>
      <tbody>
        {{range .RetryQueue}}
        <tr class="border-t border-gray-700">
          <td class="px-4 py-2 font-mono">{{.Identifier}}</td>
          <td class="px-4 py-2">{{.Attempt}}</td>
          <td class="px-4 py-2">{{.DueIn}}</td>
          <td class="px-4 py-2 text-gray-400 truncate max-w-xs">{{.Error}}</td>
        </tr>
        {{else}}
        <tr><td colspan="4" class="px-4 py-4 text-gray-500 text-center">No retries queued</td></tr>
        {{end}}
      </tbody>
    </table>
  </div>
</div>

<div>
  <h2 class="text-lg font-semibold mb-3">Recent Events</h2>
  <div class="bg-gray-800 rounded-lg overflow-hidden">
    <table class="w-full text-sm">
      <thead class="bg-gray-700">
        <tr>
          <th class="px-4 py-2 text-left">Time</th>
          <th class="px-4 py-2 text-left">Issue</th>
          <th class="px-4 py-2 text-left">Event</th>
          <th class="px-4 py-2 text-left">Message</th>
        </tr>
      </thead>
      <tbody>
        {{range .Events}}
        <tr class="border-t border-gray-700">
          <td class="px-4 py-2 text-gray-400 whitespace-nowrap">{{.Time}}</td>
          <td class="px-4 py-2 font-mono">{{.Issue}}</td>
          <td class="px-4 py-2 {{.KindColor}}">{{.Kind}}</td>
          <td class="px-4 py-2 text-gray-400 truncate max-w-md">{{.Message}}</td>
        </tr>
        {{else}}
        <tr><td colspan="4" class="px-4 py-4 text-gray-500 text-center">No events yet</td></tr>
        {{end}}
      </tbody>
    </table>
  </div>
</div>`))

type dashboardData struct {
	RunningCount   int
	MaxConcurrent  int
	RetryCount     int
	CompletedCount int
	TotalTokens    int
	Uptime         string
	StatePartial   template.HTML
}

type runningView struct {
	IssueIdentifier string
	State           string
	StateColor      string
	TurnCount       int
	Elapsed         string
	LastMessage     string
}

type retryView struct {
	Identifier string
	Attempt    int
	DueIn      string
	Error      string
}

type eventView struct {
	Time      string
	Issue     string
	Kind      string
	KindColor string
	Message   string
}

type statePartialData struct {
	Running    []runningView
	RetryQueue []retryView
	Events     []eventView
}

func stateColor(state string) string {
	switch domain.RunAttemptState(state) {
	case domain.StateStreamingTurn:
		return "text-green-400"
	case domain.StatePreparingWorkspace, domain.StateBuildingPrompt, domain.StateLaunchingAgentProcess:
		return "text-yellow-400"
	case domain.StateStalled, domain.StateFailed, domain.StateTimedOut:
		return "text-red-400"
	case domain.StateSucceeded:
		return "text-blue-400"
	default:
		return ""
	}
}

func kindColor(kind string) string {
	switch kind {
	case "dispatched":
		return "text-blue-400"
	case "turn_completed":
		return "text-green-400"
	case "worker_exit":
		return "text-yellow-400"
	case "stall_detected":
		return "text-red-400"
	case "retry_scheduled", "continuation_scheduled":
		return "text-orange-400"
	default:
		return ""
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	snap := s.state.Snapshot()

	data := dashboardData{
		RunningCount:   len(snap.Running),
		MaxConcurrent:  snap.MaxConcurrentAgents,
		RetryCount:     len(snap.RetryAttempts),
		CompletedCount: len(snap.Completed),
		TotalTokens:    snap.AgentTotals.TotalTokens,
		Uptime:         time.Since(snap.StartedAt).Round(time.Second).String(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTmpl.Execute(w, data); err != nil {
		s.logger.Error("render dashboard", "error", err)
	}
}

func (s *Server) handleIssuePage(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("identifier")
	snap := s.state.Snapshot()

	// Find in running
	for _, entry := range snap.Running {
		if entry.IssueIdentifier == identifier {
			fmt.Fprintf(w, "<h1>%s</h1><p>State: %s, Turns: %d</p>",
				entry.IssueIdentifier, entry.State, entry.TurnCount)
			return
		}
	}

	http.NotFound(w, r)
}

func (s *Server) handlePartialState(w http.ResponseWriter, _ *http.Request) {
	snap := s.state.Snapshot()
	now := time.Now()

	running := make([]runningView, 0, len(snap.Running))
	for _, entry := range snap.Running {
		lastMessage := ""
		if entry.LastMessage != nil {
			lastMessage = *entry.LastMessage
			if len(lastMessage) > 80 {
				lastMessage = lastMessage[:80] + "..."
			}
		}
		running = append(running, runningView{
			IssueIdentifier: entry.IssueIdentifier,
			State:           string(entry.State),
			StateColor:      stateColor(string(entry.State)),
			TurnCount:       entry.TurnCount,
			Elapsed:         now.Sub(entry.StartedAt).Round(time.Second).String(),
			LastMessage:     lastMessage,
		})
	}

	retryQueue := make([]retryView, 0, len(snap.RetryAttempts))
	for _, entry := range snap.RetryAttempts {
		errStr := ""
		if entry.Error != nil {
			errStr = *entry.Error
		}
		dueIn := time.Until(entry.DueAt).Round(time.Second).String()
		if entry.DueAt.Before(now) {
			dueIn = "now"
		}
		retryQueue = append(retryQueue, retryView{
			Identifier: entry.Identifier,
			Attempt:    entry.Attempt,
			DueIn:      dueIn,
			Error:      errStr,
		})
	}

	// Events: newest first
	events := make([]eventView, 0, len(snap.EventLog))
	for i := len(snap.EventLog) - 1; i >= 0; i-- {
		e := snap.EventLog[i]
		events = append(events, eventView{
			Time:      e.Timestamp.Format("15:04:05"),
			Issue:     e.Identifier,
			Kind:      e.Kind,
			KindColor: kindColor(e.Kind),
			Message:   e.Message,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	statePartialTmpl.Execute(w, statePartialData{
		Running:    running,
		RetryQueue: retryQueue,
		Events:     events,
	})
}

func (s *Server) handlePartialEvents(w http.ResponseWriter, _ *http.Request) {
	// Events are now included in the state partial, this is kept for compatibility
	s.handlePartialState(w, nil)
}
