# Symphony Go Implementation Design

**Date:** 2026-03-04
**Status:** Approved
**Spec:** https://github.com/openai/symphony/blob/main/SPEC.md

## Summary

Go implementation of the Symphony orchestration service spec, adapted to use:
- **GitHub Issues** instead of Linear as the issue tracker
- **`claude --print`** instead of the Codex app-server JSON-RPC protocol
- **Git worktrees** as the built-in workspace strategy
- **Templ + HTMX + Tailwind** for the web dashboard

Full spec conformance with these substitutions.

---

## Architecture

```
                    symphony CLI
  ┌──────────┐  ┌──────────┐  ┌───────────────────┐
  │ Workflow  │  │  Config  │  │   HTTP Server      │
  │  Loader   │──│  Layer   │  │ (Templ+HTMX+TW)   │
  │  + Watch  │  │          │  │  GET /              │
  └──────────┘  └──────────┘  │  GET /api/v1/state  │
       │             │        │  GET /api/v1/:id     │
       ▼             ▼        │  POST /api/v1/…      │
  ┌─────────────────────────┐ └──────────────────────┘
  │     Orchestrator        │◄─── reads state ────┘
  │  (single goroutine)     │
  │  owns: running, claimed │
  │  retry_attempts, totals │
  └─────┬──────────┬────────┘
        │          │
   dispatch    reconcile
        │          │
  ┌─────▼──────────▼────────┐  ┌──────────────────┐
  │   Worker Goroutines     │  │  GitHub Issues    │
  │  ┌──────────────────┐   │  │    Client         │
  │  │ Workspace Mgr    │   │  │  (gh CLI / API)   │
  │  │ (git worktree)   │   │  └──────────────────┘
  │  ├──────────────────┤   │
  │  │ Prompt Builder   │   │
  │  ├──────────────────┤   │
  │  │ Claude Runner    │   │
  │  │ (claude --print) │   │
  │  └──────────────────┘   │
  └─────────────────────────┘
```

### Concurrency model

Channel-based event loop. Single orchestrator goroutine owns all mutable state. Workers are goroutines that communicate back via a shared event channel. No locks on orchestrator state.

Event types:
- `TickEvent` - poll timer fired
- `WorkerExitEvent` - worker goroutine completed (normal or error)
- `AgentUpdateEvent` - agent activity update (for stall detection)
- `RetryTimerEvent` - retry timer fired
- `WorkflowReloadEvent` - WORKFLOW.md changed
- `RefreshRequestEvent` - HTTP API triggered immediate poll
- `ShutdownEvent` - graceful shutdown

---

## Domain Model

### Issue (normalized from GitHub Issues)

```go
type Issue struct {
    ID          string     // GitHub issue node ID
    Identifier  string     // "owner/repo#123"
    Number      int
    Title       string
    Description *string
    Priority    *int       // From P0-P4 labels
    State       string     // From symphony: labels
    BranchName  *string
    URL         *string
    Labels      []string
    BlockedBy   []BlockerRef
    CreatedAt   *time.Time
    UpdatedAt   *time.Time
}
```

### GitHub Issues state mapping

| Spec concept | GitHub mapping |
|---|---|
| `active_states` | Open issues with configured labels (e.g., `symphony:todo`, `symphony:in-progress`) |
| `terminal_states` | Closed issues, or issues with labels like `symphony:done`, `symphony:cancelled` |
| `project_slug` | `owner/repo` (config key: `tracker.repo`) |
| `priority` | Labels `P0`-`P4` → integers 0-4 |
| `blocked_by` | Issues referenced in "blocked by #N" patterns in body |

### Orchestrator state (spec Section 4.1.8)

```go
type OrchestratorState struct {
    PollIntervalMs      int
    MaxConcurrentAgents int
    Running             map[string]*RunningEntry
    Claimed             map[string]struct{}
    RetryAttempts       map[string]*RetryEntry
    Completed           map[string]struct{}
    AgentTotals         AgentTotals
    RateLimits          *RateLimitSnapshot
}
```

---

## WORKFLOW.md Configuration

Same format as spec (YAML front matter + Markdown prompt body), with adapted sections:

```yaml
---
tracker:
  kind: github
  repo: owner/repo
  api_key: $GITHUB_TOKEN
  active_states:
    - "symphony:todo"
    - "symphony:in-progress"
  terminal_states:
    - "symphony:done"
    - "symphony:cancelled"

polling:
  interval_ms: 30000

workspace:
  root: ~/symphony_workspaces
  repo_url: git@github.com:owner/repo.git
  base_branch: main

hooks:
  after_create: |
    go mod download
  before_run: |
    git pull origin main --rebase
  timeout_ms: 60000

agent:
  max_concurrent_agents: 5
  max_turns: 20
  max_retry_backoff_ms: 300000

claude:
  command: claude --print
  model: sonnet
  max_tokens: 16000
  turn_timeout_ms: 3600000
  stall_timeout_ms: 300000
  allowed_tools: []
  permission_mode: auto

server:
  port: 8080
---

You are working on issue {{ issue.identifier }}: {{ issue.title }}

{{ issue.description }}

{% if attempt %}
This is continuation attempt {{ attempt }}.
Check the current state and continue where the previous session left off.
{% endif %}
```

Key differences from spec:
- `tracker.kind: github` with `tracker.repo` instead of `project_slug`
- `claude` section replaces `codex` section
- `workspace.repo_url` and `workspace.base_branch` for git worktree support

### Dynamic reload

File watcher (fsnotify) on WORKFLOW.md. On change:
- Re-parse YAML + prompt
- Send `WorkflowReloadEvent` to orchestrator channel
- Orchestrator applies new config to future ticks
- Invalid reload keeps last good config, emits error log

---

## GitHub Issues Tracker Client

### Interface (pluggable)

```go
type TrackerClient interface {
    FetchCandidateIssues(ctx context.Context) ([]Issue, error)
    FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]Issue, error)
    FetchIssuesByStates(ctx context.Context, states []string) ([]Issue, error)
}
```

### GitHub implementation

Uses GitHub REST API (or `gh` CLI as fallback):
- **FetchCandidateIssues**: GET /repos/{owner}/{repo}/issues with label filters, paginated
- **FetchIssueStatesByIDs**: GraphQL query by node IDs or individual REST calls
- **FetchIssuesByStates**: Same as candidate but with terminal state labels + closed state

Error categories: `unsupported_tracker_kind`, `missing_tracker_api_key`, `missing_tracker_repo`, `github_api_request`, `github_api_status`, `github_graphql_errors`

---

## Workspace Manager (Git Worktrees)

### Lifecycle

1. **Setup**: Clone bare repo at `<root>/.bare` if not present
2. **Create workspace**: `git worktree add -b symphony/<key> <path> <base_branch>`
3. **Reuse workspace**: Directory exists → `created_now = false`
4. **Cleanup**: `git worktree remove <path>` + `git branch -D symphony/<key>`

### Hooks

- `after_create`: Fatal on failure (new workspace only)
- `before_run`: Fatal on failure (every attempt)
- `after_run`: Log and ignore failure
- `before_remove`: Log and ignore failure

All hooks run via `bash -lc <script>` with `cwd = workspace_path`. Timeout: `hooks.timeout_ms`.

### Safety invariants

- Workspace path must be under workspace root (absolute prefix check)
- Workspace key: only `[A-Za-z0-9._-]`, others replaced with `_`
- Claude always runs with `cwd = workspace_path`

---

## Claude Runner

### Turn execution

Each turn is a `claude --print` subprocess:

```
claude --print -p "<rendered_prompt>" [--model <model>] [--max-tokens <n>]
```

- `cwd` = workspace path
- stdout = agent response
- stderr = diagnostics (logged)
- Turn timeout via `context.WithTimeout`

### Multi-turn loop (per worker)

1. Turn 1: Full rendered prompt with issue context
2. Turn 2+: Continuation prompt referencing prior work
3. After each turn: check issue state via GitHub API
4. If still active and turns < max_turns → next turn
5. Worker exits normally → orchestrator schedules 1s continuation retry

### Stall detection

Track `started_at` per running entry. If no turn completes within `stall_timeout_ms`, kill process and retry.

### Token tracking

Wall-clock time as primary metric. Token fields remain zero unless claude CLI outputs usage info to stderr that can be parsed.

---

## HTMX Dashboard

### Pages

- `GET /` - Main dashboard (Templ + HTMX + Tailwind)
- `GET /issues/:identifier` - Issue detail page

### HTMX partials (auto-refresh)

- `GET /partials/state` - Running sessions + retry queue table
- `GET /partials/events` - Recent events list

Polling: `hx-trigger="every 3s"`

### Dashboard layout

```
┌─────────────────────────────────────────────┐
│  Symphony Dashboard              ⟳ Live     │
├─────────────────────────────────────────────┤
│  Running: 3/5    Retrying: 1    Done: 12   │
│  Runtime: 2h 34m                            │
├─────────────────────────────────────────────┤
│  Active Sessions table                      │
│  (Issue, State, Turns, Time, Status)        │
├─────────────────────────────────────────────┤
│  Retry Queue table                          │
│  (Issue, Attempt, Due In, Error)            │
├─────────────────────────────────────────────┤
│  Recent Events list                         │
│  (timestamp, issue, event, message)         │
└─────────────────────────────────────────────┘
```

### JSON API (spec Section 13.7)

- `GET /api/v1/state` - Full state snapshot
- `GET /api/v1/:identifier` - Issue detail
- `POST /api/v1/refresh` - Trigger immediate poll (202 Accepted)

---

## Project Structure

```
symphony/
├── cmd/symphony/main.go
├── internal/
│   ├── config/
│   │   ├── config.go         # Typed config layer
│   │   └── workflow.go       # WORKFLOW.md parser + watcher
│   ├── orchestrator/
│   │   ├── orchestrator.go   # Event loop + state
│   │   ├── dispatch.go       # Candidate selection + dispatch
│   │   ├── reconcile.go      # Stall detection + state refresh
│   │   └── retry.go          # Retry queue + backoff
│   ├── tracker/
│   │   ├── tracker.go        # TrackerClient interface
│   │   └── github.go         # GitHub Issues adapter
│   ├── workspace/
│   │   ├── manager.go        # Workspace lifecycle
│   │   └── hooks.go          # Hook execution
│   ├── agent/
│   │   ├── runner.go         # Claude CLI runner
│   │   └── prompt.go         # Prompt template rendering
│   ├── web/
│   │   ├── server.go         # HTTP server setup
│   │   ├── handlers.go       # Route handlers
│   │   ├── api.go            # JSON API handlers
│   │   └── templates/
│   │       ├── layout.templ
│   │       ├── dashboard.templ
│   │       ├── issue.templ
│   │       └── partials/
│   │           ├── state.templ
│   │           └── events.templ
│   └── logging/logger.go
├── go.mod
├── go.sum
├── Makefile
└── WORKFLOW.md.example
```

## CLI

```
symphony [path-to-WORKFLOW.md] [--port PORT]
```

- Positional arg: workflow file path (default: `./WORKFLOW.md`)
- `--port`: HTTP server port (overrides `server.port` in config)

## Logging

Go `log/slog` with structured JSON. Required context fields per spec:
- `issue_id`, `issue_identifier` for issue-related logs
- `session_id` for agent session logs

## Error Handling

Per spec Section 14:
- Config errors → block dispatch, log, keep service alive
- Tracker errors → skip tick, retry next tick
- Worker errors → exponential backoff retry
- Dashboard errors → log, don't crash orchestrator

## Testing

Per spec Section 17:
- Unit tests for config parsing, workspace safety, dispatch sorting, retry backoff
- Integration tests with mock GitHub API
- Table-driven tests for state machine transitions
