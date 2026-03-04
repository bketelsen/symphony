package config

import "fmt"

// Config holds the full typed configuration parsed from WORKFLOW.md.
type Config struct {
	Tracker   TrackerConfig
	Polling   PollingConfig
	Workspace WorkspaceConfig
	Hooks     HooksConfig
	Agent     AgentConfig
	Claude    ClaudeConfig
	Server    ServerConfig
}

// TrackerConfig configures the issue tracker integration.
type TrackerConfig struct {
	Kind           string   `yaml:"kind"`
	Repo           string   `yaml:"repo"`
	APIKey         string   `yaml:"api_key"`
	ActiveStates   []string `yaml:"active_states"`
	TerminalStates []string `yaml:"terminal_states"`
}

// PollingConfig configures the poll interval.
type PollingConfig struct {
	IntervalMs int `yaml:"interval_ms"`
}

// WorkspaceConfig configures git worktree workspaces.
type WorkspaceConfig struct {
	Root       string `yaml:"root"`
	RepoURL    string `yaml:"repo_url"`
	BaseBranch string `yaml:"base_branch"`
}

// HooksConfig configures lifecycle hook scripts.
type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
	TimeoutMs    int    `yaml:"timeout_ms"`
}

// AgentConfig configures agent execution limits.
type AgentConfig struct {
	MaxConcurrentAgents int `yaml:"max_concurrent_agents"`
	MaxTurns            int `yaml:"max_turns"`
	MaxRetryBackoffMs   int `yaml:"max_retry_backoff_ms"`
}

// ClaudeConfig configures the Claude CLI runner.
type ClaudeConfig struct {
	Command        string   `yaml:"command"`
	Model          string   `yaml:"model"`
	MaxTokens      int      `yaml:"max_tokens"`
	TurnTimeoutMs  int      `yaml:"turn_timeout_ms"`
	StallTimeoutMs int      `yaml:"stall_timeout_ms"`
	AllowedTools   []string `yaml:"allowed_tools"`
	PermissionMode string   `yaml:"permission_mode"`
}

// ServerConfig configures the HTTP dashboard server.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Tracker.Kind == "" {
		c.Tracker.Kind = "github"
	}
	if c.Polling.IntervalMs == 0 {
		c.Polling.IntervalMs = 30000
	}
	if c.Workspace.BaseBranch == "" {
		c.Workspace.BaseBranch = "main"
	}
	if c.Hooks.TimeoutMs == 0 {
		c.Hooks.TimeoutMs = 60000
	}
	if c.Agent.MaxConcurrentAgents == 0 {
		c.Agent.MaxConcurrentAgents = 10
	}
	if c.Agent.MaxTurns == 0 {
		c.Agent.MaxTurns = 20
	}
	if c.Agent.MaxRetryBackoffMs == 0 {
		c.Agent.MaxRetryBackoffMs = 300000
	}
	if c.Claude.Command == "" {
		c.Claude.Command = "claude --print"
	}
	if c.Claude.TurnTimeoutMs == 0 {
		c.Claude.TurnTimeoutMs = 3600000
	}
	if c.Claude.StallTimeoutMs == 0 {
		c.Claude.StallTimeoutMs = 300000
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
}

// Validate checks that required fields are present.
func (c *Config) Validate() error {
	if c.Tracker.Repo == "" {
		return fmt.Errorf("config: tracker.repo is required")
	}
	if c.Workspace.Root == "" {
		return fmt.Errorf("config: workspace.root is required")
	}
	if c.Workspace.RepoURL == "" {
		return fmt.Errorf("config: workspace.repo_url is required")
	}
	if len(c.Tracker.ActiveStates) == 0 {
		return fmt.Errorf("config: tracker.active_states must not be empty")
	}
	return nil
}
