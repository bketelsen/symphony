package config

import (
	"strings"
	"testing"
)

func TestParseWorkflowValid(t *testing.T) {
	t.Parallel()

	cfg, prompt, err := ParseWorkflowFile("testdata/valid.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tracker
	if cfg.Tracker.Kind != "github" {
		t.Errorf("tracker.kind = %q, want %q", cfg.Tracker.Kind, "github")
	}
	if cfg.Tracker.Repo != "myorg/myrepo" {
		t.Errorf("tracker.repo = %q, want %q", cfg.Tracker.Repo, "myorg/myrepo")
	}
	if cfg.Tracker.APIKey != "test-token" {
		t.Errorf("tracker.api_key = %q, want %q", cfg.Tracker.APIKey, "test-token")
	}
	if len(cfg.Tracker.ActiveStates) != 2 {
		t.Errorf("active_states len = %d, want 2", len(cfg.Tracker.ActiveStates))
	}
	if len(cfg.Tracker.TerminalStates) != 2 {
		t.Errorf("terminal_states len = %d, want 2", len(cfg.Tracker.TerminalStates))
	}

	// Polling
	if cfg.Polling.IntervalMs != 15000 {
		t.Errorf("polling.interval_ms = %d, want 15000", cfg.Polling.IntervalMs)
	}

	// Workspace
	if cfg.Workspace.Root != "/tmp/workspaces" {
		t.Errorf("workspace.root = %q, want %q", cfg.Workspace.Root, "/tmp/workspaces")
	}
	if cfg.Workspace.RepoURL != "git@github.com:myorg/myrepo.git" {
		t.Errorf("workspace.repo_url = %q", cfg.Workspace.RepoURL)
	}
	if cfg.Workspace.BaseBranch != "develop" {
		t.Errorf("workspace.base_branch = %q, want %q", cfg.Workspace.BaseBranch, "develop")
	}

	// Hooks
	if !strings.Contains(cfg.Hooks.AfterCreate, "npm install") {
		t.Errorf("hooks.after_create = %q, want to contain 'npm install'", cfg.Hooks.AfterCreate)
	}
	if cfg.Hooks.TimeoutMs != 30000 {
		t.Errorf("hooks.timeout_ms = %d, want 30000", cfg.Hooks.TimeoutMs)
	}

	// Agent
	if cfg.Agent.MaxConcurrentAgents != 3 {
		t.Errorf("agent.max_concurrent_agents = %d, want 3", cfg.Agent.MaxConcurrentAgents)
	}
	if cfg.Agent.MaxTurns != 10 {
		t.Errorf("agent.max_turns = %d, want 10", cfg.Agent.MaxTurns)
	}
	if cfg.Agent.MaxRetryBackoffMs != 120000 {
		t.Errorf("agent.max_retry_backoff_ms = %d, want 120000", cfg.Agent.MaxRetryBackoffMs)
	}

	// Claude
	if cfg.Claude.Command != "claude --print" {
		t.Errorf("claude.command = %q", cfg.Claude.Command)
	}
	if cfg.Claude.Model != "sonnet" {
		t.Errorf("claude.model = %q, want %q", cfg.Claude.Model, "sonnet")
	}
	if cfg.Claude.MaxTokens != 8000 {
		t.Errorf("claude.max_tokens = %d, want 8000", cfg.Claude.MaxTokens)
	}
	if len(cfg.Claude.AllowedTools) != 2 {
		t.Errorf("claude.allowed_tools len = %d, want 2", len(cfg.Claude.AllowedTools))
	}

	// Server
	if cfg.Server.Port != 9090 {
		t.Errorf("server.port = %d, want 9090", cfg.Server.Port)
	}

	// Prompt
	if !strings.Contains(prompt, "Fix the problem") {
		t.Errorf("prompt = %q, want to contain 'Fix the problem'", prompt)
	}
}

func TestParseWorkflowMinimalDefaults(t *testing.T) {
	t.Parallel()

	cfg, prompt, err := ParseWorkflowFile("testdata/minimal.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check defaults
	if cfg.Tracker.Kind != "github" {
		t.Errorf("default tracker.kind = %q, want %q", cfg.Tracker.Kind, "github")
	}
	if cfg.Polling.IntervalMs != 30000 {
		t.Errorf("default polling.interval_ms = %d, want 30000", cfg.Polling.IntervalMs)
	}
	if cfg.Workspace.BaseBranch != "main" {
		t.Errorf("default workspace.base_branch = %q, want %q", cfg.Workspace.BaseBranch, "main")
	}
	if cfg.Hooks.TimeoutMs != 60000 {
		t.Errorf("default hooks.timeout_ms = %d, want 60000", cfg.Hooks.TimeoutMs)
	}
	if cfg.Agent.MaxConcurrentAgents != 10 {
		t.Errorf("default agent.max_concurrent_agents = %d, want 10", cfg.Agent.MaxConcurrentAgents)
	}
	if cfg.Agent.MaxTurns != 20 {
		t.Errorf("default agent.max_turns = %d, want 20", cfg.Agent.MaxTurns)
	}
	if cfg.Agent.MaxRetryBackoffMs != 300000 {
		t.Errorf("default agent.max_retry_backoff_ms = %d, want 300000", cfg.Agent.MaxRetryBackoffMs)
	}
	if cfg.Claude.Command != "claude --print" {
		t.Errorf("default claude.command = %q", cfg.Claude.Command)
	}
	if cfg.Claude.TurnTimeoutMs != 3600000 {
		t.Errorf("default claude.turn_timeout_ms = %d, want 3600000", cfg.Claude.TurnTimeoutMs)
	}
	if cfg.Claude.StallTimeoutMs != 300000 {
		t.Errorf("default claude.stall_timeout_ms = %d, want 300000", cfg.Claude.StallTimeoutMs)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default server.port = %d, want 8080", cfg.Server.Port)
	}
	if !strings.Contains(prompt, "Issue.Title") {
		t.Errorf("prompt = %q, want to contain 'Issue.Title'", prompt)
	}
}

func TestParseWorkflowEnvVarExpansion(t *testing.T) {
	t.Setenv("SYMPHONY_TEST_TOKEN", "my-secret-token")

	input := `---
tracker:
  repo: owner/repo
  api_key: $SYMPHONY_TEST_TOKEN
  active_states:
    - "symphony:todo"

workspace:
  root: /tmp/ws
  repo_url: git@github.com:owner/repo.git
---

Prompt.
`
	cfg, _, err := ParseWorkflow(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tracker.APIKey != "my-secret-token" {
		t.Errorf("api_key = %q, want %q", cfg.Tracker.APIKey, "my-secret-token")
	}
}

func TestParseWorkflowEnvVarBraceExpansion(t *testing.T) {
	t.Setenv("SYMPHONY_TEST_TOKEN2", "braced-token")

	input := `---
tracker:
  repo: owner/repo
  api_key: ${SYMPHONY_TEST_TOKEN2}
  active_states:
    - "todo"

workspace:
  root: /tmp/ws
  repo_url: git@github.com:owner/repo.git
---

Prompt.
`
	cfg, _, err := ParseWorkflow(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tracker.APIKey != "braced-token" {
		t.Errorf("api_key = %q, want %q", cfg.Tracker.APIKey, "braced-token")
	}
}

func TestParseWorkflowInvalidYAML(t *testing.T) {
	t.Parallel()

	_, _, err := ParseWorkflowFile("testdata/invalid_yaml.md")
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "YAML") {
		t.Errorf("error = %q, want to contain 'YAML'", err.Error())
	}
}

func TestParseWorkflowMissingDelimiters(t *testing.T) {
	t.Parallel()

	input := "no front matter here"
	_, _, err := ParseWorkflow(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing delimiters, got nil")
	}
	if !strings.Contains(err.Error(), "---") {
		t.Errorf("error = %q, want to mention ---", err.Error())
	}
}

func TestParseWorkflowMissingClosingDelimiter(t *testing.T) {
	t.Parallel()

	input := "---\ntracker:\n  repo: foo\n"
	_, _, err := ParseWorkflow(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing closing ---, got nil")
	}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) {},
			wantErr: "",
		},
		{
			name:    "missing tracker.repo",
			modify:  func(c *Config) { c.Tracker.Repo = "" },
			wantErr: "tracker.repo",
		},
		{
			name:    "missing workspace.root",
			modify:  func(c *Config) { c.Workspace.Root = "" },
			wantErr: "workspace.root",
		},
		{
			name:    "missing workspace.repo_url",
			modify:  func(c *Config) { c.Workspace.RepoURL = "" },
			wantErr: "workspace.repo_url",
		},
		{
			name:    "empty active_states",
			modify:  func(c *Config) { c.Tracker.ActiveStates = nil },
			wantErr: "active_states",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func validConfig() Config {
	return Config{
		Tracker: TrackerConfig{
			Repo:         "owner/repo",
			ActiveStates: []string{"symphony:todo"},
		},
		Workspace: WorkspaceConfig{
			Root:    "/tmp/ws",
			RepoURL: "git@github.com:owner/repo.git",
		},
	}
}
