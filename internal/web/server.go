package web

import (
	"log/slog"
	"net/http"

	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/orchestrator"
)

// StateProvider provides orchestrator state to the web layer.
type StateProvider interface {
	Snapshot() orchestrator.OrchestratorState
	Events() chan<- domain.Event
}

// Server is the HTTP dashboard and API server.
type Server struct {
	state  StateProvider
	mux    *http.ServeMux
	logger *slog.Logger
}

// NewServer creates a new web server.
func NewServer(state StateProvider, logger *slog.Logger) *Server {
	s := &Server{
		state:  state,
		mux:    http.NewServeMux(),
		logger: logger,
	}
	s.routes()
	return s
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// Pages
	s.mux.HandleFunc("GET /", s.handleDashboard)
	s.mux.HandleFunc("GET /issues/{identifier}", s.handleIssuePage)

	// HTMX partials
	s.mux.HandleFunc("GET /partials/state", s.handlePartialState)
	s.mux.HandleFunc("GET /partials/events", s.handlePartialEvents)

	// JSON API
	s.mux.HandleFunc("GET /api/v1/state", s.handleAPIState)
	s.mux.HandleFunc("GET /api/v1/issues/{identifier}", s.handleAPIIssue)
	s.mux.HandleFunc("POST /api/v1/refresh", s.handleAPIRefresh)
}
