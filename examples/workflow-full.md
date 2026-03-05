---
# symphony WORKFLOW.md — fully annotated example
#
# WORKFLOW.md is the single configuration file for a symphony project.
# It uses YAML front matter (between --- fences) for settings, followed
# by a Go text/template that becomes the agent prompt for each issue.
#
# Environment variables are expanded in the YAML block using $VAR or ${VAR}.
# All fields below match the struct tags in internal/config/config.go.

# ─── tracker ─────────────────────────────────────────────────────────────────
# Configures the issue tracker integration.
tracker:
  # kind: only "github" is supported today.
  kind: github

  # repo: the GitHub repository to poll for issues, in "owner/repo" format.
  # Required.
  repo: myorg/myrepo

  # api_key: GitHub personal access token (or fine-grained token) with
  # "Issues: read" and "Pull requests: write" scopes.
  # Optional — defaults to the $GITHUB_TOKEN environment variable.
  # Prefer the env-var form so you don't commit secrets.
  api_key: ${GITHUB_TOKEN}

  # active_states: list of GitHub labels that mark an issue as ready for
  # symphony to pick up.  An issue must carry at least one of these labels
  # to enter the work queue.
  active_states:
    - "symphony:todo"
    - "symphony:in-progress"

  # terminal_states: labels that mean the issue is finished.  When symphony
  # merges a PR it applies one of these labels, removing active_states labels.
  terminal_states:
    - "symphony:done"
    - "symphony:cancelled"

# ─── polling ─────────────────────────────────────────────────────────────────
# Controls how often symphony checks the tracker for new or changed issues.
polling:
  # interval_ms: milliseconds between GitHub API polls.
  # Default: 30000 (30 s).  Lower values mean faster pickup but more API calls.
  interval_ms: 15000

# ─── workspace ───────────────────────────────────────────────────────────────
# Configures where symphony creates git worktrees for each issue.
workspace:
  # root: absolute path on disk where per-issue worktrees are created.
  # Each issue gets its own subdirectory: <root>/<repo-slug>/<issue-number>/.
  # Required.
  root: /var/symphony/workspaces

  # repo_url: SSH or HTTPS URL symphony uses when cloning the repository into
  # a new worktree.  Required.
  repo_url: git@github.com:myorg/myrepo.git

  # base_branch: the branch that new worktrees are branched from.
  # Default: "main".
  base_branch: main

# ─── hooks ───────────────────────────────────────────────────────────────────
# Shell snippets that run at lifecycle events for each worktree.
# Hooks execute with the worktree directory as the working directory.
# They have access to the issue identifier via the SYMPHONY_ISSUE env var.
#
# Failure behaviour:
#   after_create  — FATAL: if this exits non-zero, the workspace is torn down
#                   and the issue is retried.  Use it for mandatory setup steps
#                   (installing deps, running migrations, etc.).
#   before_run    — FATAL: if this exits non-zero, the agent is not launched
#                   and the issue is retried.  Use it for mandatory pre-flight
#                   checks (linting, formatting, secrets injection, etc.).
#   after_run     — non-fatal: exit code is logged but does not affect retries.
#                   Use it for post-processing (uploading artefacts, notifying
#                   Slack, etc.).
#   before_remove — non-fatal: exit code is logged.  Use it for cleanup
#                   (deleting temp files, stopping local services, etc.).
hooks:
  # after_create: runs once, immediately after the git worktree is created
  # and before the agent first starts.  Ideal for installing dependencies.
  after_create: |
    set -euo pipefail
    go mod download
    npm ci --prefix frontend

  # before_run: runs before every agent turn (including retries and
  # continuations).  Ideal for pulling latest changes and validating state.
  before_run: |
    set -euo pipefail
    git fetch origin
    git rebase origin/main

  # after_run: runs after each agent turn completes (success or failure).
  # Useful for notifications or artefact uploads.
  after_run: |
    echo "Turn finished for $SYMPHONY_ISSUE"

  # before_remove: runs before the worktree is deleted.  Use for cleanup.
  before_remove: |
    docker compose -f docker-compose.dev.yml down --remove-orphans || true

  # timeout_ms: maximum wall-clock time any single hook invocation may run.
  # Default: 60000 (60 s).
  timeout_ms: 120000

# ─── agent ───────────────────────────────────────────────────────────────────
# Limits and tuning for the agent execution loop.
agent:
  # max_concurrent_agents: how many issues symphony works on simultaneously.
  # Set this to a number your machine and GitHub rate limits can support.
  # Default: 10.
  max_concurrent_agents: 3

  # max_turns: maximum number of Claude turns per issue before symphony
  # considers the run exhausted and marks it for retry.
  # Default: 20.
  max_turns: 15

  # max_retry_backoff_ms: cap on the exponential back-off when an agent fails.
  # Back-off starts at 10 s and doubles each attempt, up to this ceiling.
  # Default: 300000 (5 min).
  max_retry_backoff_ms: 600000

  # idle_before_ready_ms: how long symphony waits after the workspace is ready
  # before it considers the agent "idle" and eligible to start the next turn.
  # Increase this if your after_create hook starts background processes that
  # need a moment to stabilise.
  # Default: 60000 (60 s).
  idle_before_ready_ms: 30000

# ─── claude ──────────────────────────────────────────────────────────────────
# Configures the Claude CLI invocation.
claude:
  # command: the shell command used to invoke Claude.  "--print" is required
  # for non-interactive streaming output.
  # Default: "claude --print".
  command: claude --print

  # model: Claude model identifier passed via --model.  Use the latest Sonnet
  # for a good balance of capability and cost; Opus for harder tasks.
  # Default: none (uses Claude CLI default).
  model: claude-sonnet-4-6

  # max_tokens: maximum output tokens per turn.  0 means no explicit cap.
  # Default: 0.
  max_tokens: 16000

  # turn_timeout_ms: maximum wall-clock time for a single Claude turn.
  # If the Claude process does not finish within this window symphony cancels
  # the turn and marks it as timed out.
  # Default: 3600000 (1 hour).
  turn_timeout_ms: 1800000

  # stall_timeout_ms: maximum time without any output from the Claude process
  # before symphony considers it stalled and cancels the turn.
  # Default: 300000 (5 min).
  stall_timeout_ms: 120000

  # allowed_tools: list of Claude tools the agent is permitted to use.
  # Omit to allow all tools.  Use this to restrict risky capabilities.
  allowed_tools:
    - Bash
    - Read
    - Write
    - Edit
    - Glob
    - Grep

  # permission_mode: controls Claude's permission prompt behaviour.
  # "auto"        — Claude auto-approves safe operations (recommended for CI).
  # "default"     — Claude uses its built-in defaults.
  # "bypassPerms" — Claude skips all permission checks (use with caution).
  permission_mode: auto

# ─── server ──────────────────────────────────────────────────────────────────
# Configures the built-in HTTP dashboard server.
server:
  # port: TCP port the dashboard listens on.  Browse to http://localhost:<port>
  # to see live agent status, event logs, and token usage.
  # Default: 8080.
  port: 8080

---
# ─── Prompt Template ─────────────────────────────────────────────────────────
#
# Everything below the closing --- is a Go text/template rendered for each
# issue before the agent starts.  The template receives a PromptData value
# (see internal/agent/prompt.go) with the following fields:
#
#   .Issue.Identifier  — "owner/repo#123"
#   .Issue.Title       — issue title string
#   .Issue.Description — *string (pointer, may be nil — use the `deref` func)
#   .Attempt           — int, 0 on the first attempt, 1+ on retries
#
# Template functions available:
#   deref <*string>  — safely dereference a string pointer; returns "" if nil
#
# Tips:
#   • Keep the first line short — it becomes the branch name prefix.
#   • Use {{ if }} blocks to vary instructions based on context.
#   • Custom instructions go here, not in config — this is the agent's brief.

You are working on {{ .Issue.Identifier }}: {{ .Issue.Title }}

{{ if .Issue.Description -}}
## Issue description

{{ deref .Issue.Description }}
{{ end -}}

{{ if gt .Attempt 0 -}}
## Continuation (attempt {{ .Attempt }})

This is not your first attempt at this issue.  Review the current state of the
branch before starting work:

1. Run `git log --oneline -10` to see what was committed in previous attempts.
2. Run `git status` to check for any uncommitted changes.
3. Continue from where the previous attempt left off rather than starting over.

{{ end -}}

## Your task

Implement the changes described in the issue above.

## Engineering guidelines

- Write tests first (TDD).  All new behaviour must be covered by tests.
- Run the full test suite before committing: `go test ./...`
- Use conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`
- Create all pull requests as **drafts** using `gh pr create --draft`.
- Keep pull requests focused — one logical change per PR.
- Do not modify files unrelated to the issue.

## Definition of done

1. All tests pass (`go test ./...`).
2. `golangci-lint run ./...` produces no new warnings.
3. A draft PR is open against `main` with a clear description.
4. The PR description references this issue with "Closes {{ .Issue.Identifier }}".
