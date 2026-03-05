# Symphony `init` Command

## Goal

Add a `symphony init` CLI subcommand that generates a complete `WORKFLOW.md` by introspecting the current git repo's GitHub remote, then ensures required labels exist in the repo.

## Architecture

Pure CLI, no interactive prompts. Introspect via `gh repo view --json`, generate WORKFLOW.md with sensible defaults, create missing GitHub labels.

Follows the same pattern as `symphony create-issues`: new file in `cmd/symphony/`, subcommand routing in `main.go`, uses `execCommandRunner` for `gh` calls.

## Introspection

Via `gh repo view --json owner,name,sshUrl,defaultBranchRef`:

| Config field | Source | Fallback |
|---|---|---|
| `tracker.repo` | `owner.login/name` | required |
| `workspace.repo_url` | `sshUrl` | required |
| `workspace.base_branch` | `defaultBranchRef.name` | `main` |

All other fields use existing `ApplyDefaults()` values from `config.go`.

## Labels

After writing WORKFLOW.md, check for and create missing labels:

| Label | Color | Purpose |
|---|---|---|
| `symphony:todo` | `0e8a16` (green) | Active state — new work |
| `symphony:in-progress` | `fbca04` (yellow) | Active state — dispatched |
| `symphony:done` | `0075ca` (blue) | Terminal state — complete |
| `symphony:cancelled` | `cccccc` (gray) | Terminal state — cancelled |
| `P0` | `d93f0b` (red) | Priority 0 |
| `P1` | `fbca04` (yellow) | Priority 1 |
| `P2` | `0075ca` (blue) | Priority 2 |

Uses `gh label list --json name` to check existing, only creates missing ones via `gh label create`.

## CLI Interface

```
symphony init [flags]
```

**Flags:**
- `--workspace-root <path>` — override workspace root (default `/tmp/symphony_workspaces`)
- `--force` — overwrite existing WORKFLOW.md

**Guards:**
- Error if not in a git repo
- Error if no GitHub remote found (gh fails)
- Error if WORKFLOW.md already exists (unless `--force`)

## Generated WORKFLOW.md

Standard template with introspected values filled in:

```yaml
---
tracker:
  kind: github
  repo: <owner/name>
  active_states:
    - "symphony:todo"
    - "symphony:in-progress"
  terminal_states:
    - "symphony:done"
    - "symphony:cancelled"

polling:
  interval_ms: 30000

workspace:
  root: <workspace-root>
  repo_url: <sshUrl>
  base_branch: <defaultBranch>

hooks:
  before_run: |
    git pull origin <defaultBranch> --rebase
  timeout_ms: 60000

agent:
  max_concurrent_agents: 2
  max_turns: 10
  max_retry_backoff_ms: 300000

claude:
  command: claude --print
  model: sonnet
  turn_timeout_ms: 3600000
  stall_timeout_ms: 300000
  permission_mode: bypassPermissions

server:
  port: 8080
---

You are working on issue {{ .Issue.Identifier }}: {{ .Issue.Title }}

{{ if .Issue.Description }}{{ deref .Issue.Description }}{{ end }}

When creating pull requests, always create them as drafts using `gh pr create --draft`.

{{ if gt .Attempt 0 }}
This is continuation attempt {{ .Attempt }}.
Check the current state and continue where the previous session left off.
{{ end }}
```

## Output

```
Detected repo: owner/repo
Default branch: main
Clone URL: git@github.com:owner/repo.git

Created WORKFLOW.md
Created label: symphony:todo
Created label: symphony:in-progress
Label exists: symphony:done (skipped)
...

Ready! Edit WORKFLOW.md to customize, then run: symphony
```

## Files

- Create: `cmd/symphony/init.go` — `runInit` function
- Create: `cmd/symphony/init_test.go` — tests with mock command runner
- Modify: `cmd/symphony/main.go` — add `case "init"` to subcommand routing
