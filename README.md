# Symphony

A Go implementation of the [OpenAI Symphony](https://github.com/openai/symphony/blob/main/SPEC.md) orchestration spec, adapted for GitHub Issues and Claude Code.

## Overview

Symphony watches your GitHub Issues, dispatches Claude Code agents in isolated git worktrees, and tracks progress on a live HTMX dashboard. Each agent works on a single issue in its own worktree — parallel, isolated, and fully automated.

This implementation follows the [Symphony spec](https://github.com/openai/symphony/blob/main/SPEC.md) with targeted adaptations: GitHub Issues replaces Linear as the issue tracker, `claude --print` replaces the Codex JSON-RPC protocol, and git worktrees provide built-in workspace isolation. The dashboard is built with Templ, HTMX, and Tailwind instead of a React SPA.

## Spec Comparison

| Spec Concept | OpenAI Symphony | This Implementation |
|---|---|---|
| Issue tracker | Linear | GitHub Issues |
| Agent runtime | Codex JSON-RPC | `claude --print` CLI |
| Workspace strategy | Generic | Git worktrees |
| State labels | Linear states | `symphony:todo`, `symphony:in-progress`, etc. |
| Dashboard | React SPA | Templ + HTMX + Tailwind |
| Configuration | YAML config | WORKFLOW.md (YAML front matter + prompt template) |
| Blocking | Linear relations | `blocked by #N` in issue body |
| Priority | Linear priority field | `P0`-`P4` labels |

## Quick Start

**Prerequisites:**
- Go 1.22+
- [`gh`](https://cli.github.com/) CLI authenticated to your account
- [`claude`](https://claude.ai/code) CLI installed and authenticated

**Install:**

```bash
go install github.com/bketelsen/symphony/cmd/symphony@latest
```

**Initialize your repository:**

```bash
cd your-project
symphony init
```

This creates GitHub labels (`symphony:todo`, `symphony:in-progress`, etc.) and a `WORKFLOW.md` configuration file.

**Create an issue:**

```bash
gh issue create --title "Add a README" --label "symphony:todo" \
  --body "Create a README.md with project description and usage examples."
```

**Run symphony:**

```bash
symphony
```

**Open the dashboard:** `http://localhost:8080`

See [examples/quickstart.md](examples/quickstart.md) for a full step-by-step walkthrough.

## CLI Reference

| Command | Description |
|---|---|
| `symphony` | Start the orchestrator using `./WORKFLOW.md` |
| `symphony [path]` | Start using a specific WORKFLOW.md path |
| `symphony --port PORT` | Override the dashboard port |
| `symphony --version` | Print version and exit |
| `symphony init` | Create GitHub labels and generate WORKFLOW.md |
| `symphony create-issues [flags] <plan.md>` | Convert a markdown plan into GitHub Issues |

**`create-issues` flags:**

| Flag | Default | Description |
|---|---|---|
| `--workflow PATH` | `./WORKFLOW.md` | Path to WORKFLOW.md |
| `--dry-run` | false | Print what would be created without creating issues |
| `--plan-ref URL` | — | URL or path to the full plan, linked in each issue body |

## Configuration

`WORKFLOW.md` has two parts: a YAML front matter block (configuration) and a Markdown body (the agent prompt template). Symphony watches this file for changes and reloads configuration on the fly without restarting.

Example front matter:

```yaml
---
tracker:
  kind: github
  repo: owner/repo
  active_states:
    - "symphony:todo"
  terminal_states:
    - "symphony:done"

workspace:
  root: /tmp/symphony-workspaces
  repo_url: git@github.com:owner/repo.git
  base_branch: main

claude:
  command: claude --print
  model: sonnet
  max_concurrent_agents: 5

server:
  port: 8080
---

You are working on issue {{ .Issue.Identifier }}: {{ .Issue.Title }}

{{ deref .Issue.Description }}
```

See [examples/workflow-minimal.md](examples/workflow-minimal.md) for the minimal required configuration and [examples/workflow-full.md](examples/workflow-full.md) for all available options.

## Dashboard

Symphony serves a live dashboard at `http://localhost:8080` that updates automatically every 3 seconds via HTMX polling.

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

**JSON API:**

| Endpoint | Description |
|---|---|
| `GET /api/v1/state` | Full orchestrator state snapshot |
| `GET /api/v1/:identifier` | Issue detail |
| `POST /api/v1/refresh` | Trigger an immediate poll (202 Accepted) |

## Claude Code Integration

Symphony includes a Claude Code skill (`symphony-create-issues`) for converting implementation plans into GitHub Issues ready for orchestration. Write a plan in Claude Code, run the skill, and Symphony picks up the issues automatically on its next poll cycle.

See [docs/symphony-create-issues.md](docs/symphony-create-issues.md) for full documentation on the skill, input format, and examples.

## Examples

- [examples/quickstart.md](examples/quickstart.md) — End-to-end walkthrough from install to running agent
- [examples/workflow-minimal.md](examples/workflow-minimal.md) — Minimal WORKFLOW.md configuration
- [examples/workflow-full.md](examples/workflow-full.md) — Full WORKFLOW.md with all options documented
- [examples/github-setup.md](examples/github-setup.md) — GitHub label setup and branch protection configuration

## License

MIT License — see [LICENSE](LICENSE).
