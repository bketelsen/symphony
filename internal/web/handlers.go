package web

import (
	"fmt"
	"html/template"
	"net/http"
	"time"
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
  <div class="max-w-6xl mx-auto px-4 py-6">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">Symphony Dashboard</h1>
      <span class="text-green-400 text-sm animate-pulse">&#x25CF; Live</span>
    </div>

    <div class="grid grid-cols-4 gap-4 mb-6">
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
          <th class="px-4 py-2 text-left">Last Event</th>
        </tr>
      </thead>
      <tbody>
        {{range .Running}}
        <tr class="border-t border-gray-700">
          <td class="px-4 py-2 font-mono">{{.IssueIdentifier}}</td>
          <td class="px-4 py-2">{{.State}}</td>
          <td class="px-4 py-2">{{.TurnCount}}</td>
          <td class="px-4 py-2">{{.Elapsed}}</td>
          <td class="px-4 py-2 text-gray-400 truncate max-w-xs">{{.LastEvent}}</td>
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
</div>`))

var eventsPartialTmpl = template.Must(template.New("events").Parse(`
<div>
  <h2 class="text-lg font-semibold mb-3">Recent Events</h2>
  <div class="bg-gray-800 rounded-lg p-4">
    <p class="text-gray-500 text-sm">Event log coming soon</p>
  </div>
</div>`))

type dashboardData struct {
	RunningCount   int
	MaxConcurrent  int
	RetryCount     int
	CompletedCount int
	Uptime         string
	StatePartial   template.HTML
}

type runningView struct {
	IssueIdentifier string
	State           string
	TurnCount       int
	Elapsed         string
	LastEvent       string
}

type retryView struct {
	Identifier string
	Attempt    int
	DueIn      string
	Error      string
}

type statePartialData struct {
	Running    []runningView
	RetryQueue []retryView
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	snap := s.state.Snapshot()

	data := dashboardData{
		RunningCount:   len(snap.Running),
		MaxConcurrent:  snap.MaxConcurrentAgents,
		RetryCount:     len(snap.RetryAttempts),
		CompletedCount: len(snap.Completed),
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
		lastEvent := ""
		if entry.LastEvent != nil {
			lastEvent = *entry.LastEvent
		}
		running = append(running, runningView{
			IssueIdentifier: entry.IssueIdentifier,
			State:           string(entry.State),
			TurnCount:       entry.TurnCount,
			Elapsed:         now.Sub(entry.StartedAt).Round(time.Second).String(),
			LastEvent:       lastEvent,
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	statePartialTmpl.Execute(w, statePartialData{
		Running:    running,
		RetryQueue: retryQueue,
	})
}

func (s *Server) handlePartialEvents(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	eventsPartialTmpl.Execute(w, nil)
}
