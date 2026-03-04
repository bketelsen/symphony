package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/domain"
	"github.com/bjk/symphony/internal/tracker"
)

// ProcessRunner starts external processes.
type ProcessRunner interface {
	Start(ctx context.Context, cmd string, args []string, dir string) (Process, error)
}

// Process represents a running external process.
type Process interface {
	Wait() error
	Stdout() io.Reader
	Stderr() io.Reader
}

// Runner executes claude CLI turns and multi-turn sessions.
type Runner struct {
	command        string
	model          string
	maxTokens      int
	turnTimeoutMs  int
	allowedTools   []string
	permissionMode string
	processRunner  ProcessRunner
	logger         *slog.Logger
}

// NewRunner creates an agent runner from config.
func NewRunner(cfg config.ClaudeConfig, pr ProcessRunner, logger *slog.Logger) *Runner {
	return &Runner{
		command:        cfg.Command,
		model:          cfg.Model,
		maxTokens:      cfg.MaxTokens,
		turnTimeoutMs:  cfg.TurnTimeoutMs,
		allowedTools:   cfg.AllowedTools,
		permissionMode: cfg.PermissionMode,
		processRunner:  pr,
		logger:         logger,
	}
}

// RunTurn executes a single claude --print turn.
// Returns the stdout output and any error.
func (r *Runner) RunTurn(ctx context.Context, prompt string, workDir string) (string, error) {
	timeout := time.Duration(r.turnTimeoutMs) * time.Millisecond
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	args := r.buildArgs(prompt)

	proc, err := r.processRunner.Start(ctx, r.commandName(), args, workDir)
	if err != nil {
		return "", fmt.Errorf("agent: start process: %w", err)
	}

	stdout, err := io.ReadAll(proc.Stdout())
	if err != nil {
		return "", fmt.Errorf("agent: read stdout: %w", err)
	}

	// Read stderr for diagnostics (non-fatal)
	stderrData, _ := io.ReadAll(proc.Stderr())
	if len(stderrData) > 0 {
		r.logger.Debug("claude stderr", "output", string(stderrData))
	}

	if err := proc.Wait(); err != nil {
		return string(stdout), fmt.Errorf("agent: process exited: %w", err)
	}

	return string(stdout), nil
}

// SessionParams holds parameters for a multi-turn session.
type SessionParams struct {
	Issue          domain.Issue
	PromptTemplate string
	Attempt        int
	WorkDir        string
	MaxTurns       int
	Tracker        tracker.TrackerClient
	Updates        chan<- domain.Event
	SessionID      string
}

// RunSession executes a multi-turn agent loop for an issue.
func (r *Runner) RunSession(ctx context.Context, params SessionParams) error {
	for turn := 1; turn <= params.MaxTurns; turn++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Build prompt
		var prompt string
		var err error
		if turn == 1 {
			prompt, err = RenderPrompt(params.PromptTemplate, PromptData{
				Issue:   params.Issue,
				Attempt: params.Attempt,
			})
			if err != nil {
				return fmt.Errorf("agent: render prompt: %w", err)
			}
		} else {
			prompt = BuildContinuationPrompt(params.Issue, turn)
		}

		// Execute turn
		output, err := r.RunTurn(ctx, prompt, params.WorkDir)

		// Send update event
		if params.Updates != nil {
			params.Updates <- domain.AgentUpdateEvent{
				IssueID:     params.Issue.ID,
				TurnCount:   turn,
				LastEvent:    "turn_completed",
				LastMessage:  truncate(output, 200),
			}
		}

		if err != nil {
			return fmt.Errorf("agent: turn %d: %w", turn, err)
		}

		// Check if issue is still active
		if params.Tracker != nil {
			issues, err := params.Tracker.FetchIssueStatesByIDs(ctx, []string{params.Issue.ID})
			if err != nil {
				r.logger.Warn("failed to check issue state", "error", err, "issue_id", params.Issue.ID)
				continue // keep going if we can't check
			}
			if len(issues) > 0 {
				issue := issues[0]
				// If issue is no longer in an active-looking state, stop
				if issue.State != params.Issue.State {
					r.logger.Info("issue state changed, ending session",
						"issue_id", params.Issue.ID,
						"old_state", params.Issue.State,
						"new_state", issue.State,
					)
					return nil
				}
			}
		}
	}

	r.logger.Info("max turns reached", "issue_id", params.Issue.ID, "max_turns", params.MaxTurns)
	return nil
}

func (r *Runner) commandName() string {
	parts := strings.Fields(r.command)
	if len(parts) > 0 {
		return parts[0]
	}
	return "claude"
}

func (r *Runner) buildArgs(prompt string) []string {
	parts := strings.Fields(r.command)
	var args []string
	if len(parts) > 1 {
		args = append(args, parts[1:]...)
	}

	args = append(args, "-p", prompt)

	if r.model != "" {
		args = append(args, "--model", r.model)
	}
	if r.maxTokens > 0 {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", r.maxTokens))
	}
	if r.permissionMode != "" {
		args = append(args, "--permission-mode", r.permissionMode)
	}
	for _, tool := range r.allowedTools {
		args = append(args, "--allowedTools", tool)
	}

	return args
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
