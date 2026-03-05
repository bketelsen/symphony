package domain

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"
)

// Issue represents a normalized issue from the tracker.
type Issue struct {
	ID          string
	Identifier  string // "owner/repo#123"
	Number      int
	Title       string
	Description *string
	Priority    *int // From P0-P4 labels; nil sorts last
	State       string
	BranchName  *string
	URL         *string
	Labels      []string
	BlockedBy   []BlockerRef
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

// BlockerRef references another issue that blocks this one.
type BlockerRef struct {
	Identifier string
	ID         string
}

// IsBlocked returns true if any blocker is not in the completed set.
func (i Issue) IsBlocked(completed map[string]struct{}) bool {
	for _, b := range i.BlockedBy {
		if _, done := completed[b.ID]; !done {
			return true
		}
	}
	return false
}

// SortCandidates sorts issues by priority asc (nil last), created_at asc, identifier asc.
func SortCandidates(issues []Issue) {
	slices.SortStableFunc(issues, func(a, b Issue) int {
		// Priority: ascending, nil sorts last
		switch {
		case a.Priority == nil && b.Priority == nil:
			// fall through
		case a.Priority == nil:
			return 1
		case b.Priority == nil:
			return -1
		case *a.Priority != *b.Priority:
			return *a.Priority - *b.Priority
		}

		// CreatedAt: ascending (oldest first)
		if a.CreatedAt != nil && b.CreatedAt != nil {
			if !a.CreatedAt.Equal(*b.CreatedAt) {
				if a.CreatedAt.Before(*b.CreatedAt) {
					return -1
				}
				return 1
			}
		}

		// Identifier: lexicographic
		return strings.Compare(a.Identifier, b.Identifier)
	})
}

// RunAttemptState represents the state of a running agent attempt.
type RunAttemptState string

const (
	StatePreparingWorkspace       RunAttemptState = "PreparingWorkspace"
	StateBuildingPrompt           RunAttemptState = "BuildingPrompt"
	StateLaunchingAgentProcess    RunAttemptState = "LaunchingAgentProcess"
	StateStreamingTurn            RunAttemptState = "StreamingTurn"
	StateFinishing                RunAttemptState = "Finishing"
	StateSucceeded                RunAttemptState = "Succeeded"
	StateFailed                   RunAttemptState = "Failed"
	StateTimedOut                 RunAttemptState = "TimedOut"
	StateStalled                  RunAttemptState = "Stalled"
	StateCanceledByReconciliation RunAttemptState = "CanceledByReconciliation"
)

// RunningEntry tracks a currently running agent session.
type RunningEntry struct {
	IssueID         string
	IssueIdentifier string
	State           RunAttemptState
	SessionID       string
	TurnCount       int
	StartedAt       time.Time
	LastEventAt     *time.Time
	LastEvent       *string
	LastMessage     *string
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	Cancel          context.CancelFunc `json:"-"`
}

// RetryEntry represents an issue waiting to be retried.
type RetryEntry struct {
	IssueID        string
	Identifier     string
	Attempt        int
	DueAt          time.Time
	Error          *string
	IsContinuation bool
}

// IsReady returns true if the retry is due.
func (r RetryEntry) IsReady(now time.Time) bool {
	return !now.Before(r.DueAt)
}

// AgentTotals tracks aggregate resource usage.
type AgentTotals struct {
	InputTokens    int
	OutputTokens   int
	TotalTokens    int
	SecondsRunning float64
}

// CalculateBackoff returns the retry delay for a given attempt.
// Continuations always use 1s. Failures use exponential backoff: min(10000ms * 2^(attempt-1), maxBackoffMs).
func CalculateBackoff(attempt int, isContinuation bool, maxBackoffMs int) time.Duration {
	if isContinuation {
		return time.Second
	}
	if attempt < 1 {
		attempt = 1
	}
	ms := 10000.0 * math.Pow(2, float64(attempt-1))
	if ms > float64(maxBackoffMs) {
		ms = float64(maxBackoffMs)
	}
	return time.Duration(ms) * time.Millisecond
}

// AwaitingMergeEntry tracks an issue whose PR has been undrafted, waiting for merge.
type AwaitingMergeEntry struct {
	IssueID    string
	Identifier string
	Number     int
}

// EventLogEntry records a notable event for the dashboard.
type EventLogEntry struct {
	Timestamp  time.Time
	IssueID    string
	Identifier string
	Kind       string // "dispatched", "turn_completed", "worker_exit", "retry_scheduled", "stall_detected", "label_updated", "pr_ready"
	Message    string
}

// Event is the interface for all orchestrator events.
type Event interface {
	eventTag()
}

// TickEvent fires on each poll interval.
type TickEvent struct{}

func (TickEvent) eventTag() {}

// WorkerExitEvent signals a worker goroutine has completed.
type WorkerExitEvent struct {
	IssueID        string
	Err            error // nil = normal exit
	IsContinuation bool
}

func (WorkerExitEvent) eventTag() {}

// AgentUpdateEvent carries per-turn agent activity updates.
type AgentUpdateEvent struct {
	IssueID      string
	TurnCount    int
	LastEvent    string
	LastMessage  string
	InputTokens  int
	OutputTokens int
}

func (AgentUpdateEvent) eventTag() {}

// RetryTimerEvent fires when a retry timer expires.
type RetryTimerEvent struct {
	IssueID string
}

func (RetryTimerEvent) eventTag() {}

// WorkflowReloadEvent signals WORKFLOW.md has changed.
type WorkflowReloadEvent struct{}

func (WorkflowReloadEvent) eventTag() {}

// RefreshRequestEvent triggers an immediate poll cycle.
type RefreshRequestEvent struct {
	ReplyCh chan<- struct{}
}

func (RefreshRequestEvent) eventTag() {}

// ShutdownEvent requests graceful shutdown.
type ShutdownEvent struct{}

func (ShutdownEvent) eventTag() {}
