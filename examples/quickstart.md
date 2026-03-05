# Quickstart Guide

This walkthrough takes you from zero to a running symphony agent in under five minutes.

## Prerequisites

- Go 1.22+
- [GitHub CLI](https://cli.github.com/) (`gh`) authenticated to your account
- [Claude CLI](https://claude.ai/code) (`claude`) installed and authenticated

---

## Step 1: Install

```
$ go install github.com/bketelsen/symphony/cmd/symphony@latest
```

Verify the install:

```
$ symphony --version
symphony v0.1.0
```

---

## Step 2: Navigate to your repo

```
$ cd your-project
```

Symphony reads the GitHub remote from your repo's git config, so run all commands from inside your project directory.

---

## Step 3: Initialize

```
$ symphony init
```

Expected output:

```
Detected GitHub repository: owner/your-project
Creating labels...
  created: symphony:todo
  created: symphony:in-progress
  created: symphony:done
  created: symphony:cancelled
  created: P0
  created: P1
  created: P2
Labels created.
Writing WORKFLOW.md...
  wrote: WORKFLOW.md
Done. Edit WORKFLOW.md to customize your agent prompt.
```

This creates three things:

1. **GitHub labels** — `symphony:todo`, `symphony:in-progress`, `symphony:done`, `symphony:cancelled`, and priority labels `P0`–`P2`.
2. **`WORKFLOW.md`** — your agent configuration file, committed to the repo root.

---

## Step 4: Review and customize WORKFLOW.md

Open the generated `WORKFLOW.md`:

```
$ cat WORKFLOW.md
```

```markdown
---
tracker:
  kind: github
  repo: owner/your-project
  active_states:
    - "symphony:todo"
  terminal_states:
    - "symphony:done"

workspace:
  root: /tmp/symphony-workspaces
  repo_url: git@github.com:owner/your-project.git
  base_branch: main

claude:
  command: claude --print
---

You are working on issue {{ .Issue.Identifier }}: {{ .Issue.Title }}

{{ deref .Issue.Description }}

When your work is complete, create a draft pull request using:
  gh pr create --draft --title "{{ .Issue.Title }}" --body "Closes #{{ .Issue.Number }}"
```

Things to customize before running:

| Field | What to change |
|-------|---------------|
| `workspace.root` | Where symphony checks out worktrees. Pick a path with enough disk space. |
| `workspace.repo_url` | Use HTTPS (`https://github.com/...`) if you prefer that over SSH. |
| `workspace.base_branch` | Change from `main` if your default branch is different (e.g., `master`). |
| `claude.command` | Add flags like `--model claude-opus-4-6` to choose a specific model. |
| Agent prompt (body) | Add project-specific instructions — coding conventions, test requirements, PR checklist. |

---

## Step 5: Create a test issue

```
$ gh issue create \
    --title "Add a README" \
    --label "symphony:todo" \
    --body "Create a README.md for this project. Include: project name and description, installation instructions, and a usage example."
```

Expected output:

```
Creating issue in owner/your-project

https://github.com/owner/your-project/issues/1
```

The `symphony:todo` label tells symphony this issue is ready to be picked up.

---

## Step 6: Start symphony

```
$ symphony
```

Expected log output:

```
2026/03/04 12:00:00 INFO dashboard starting address=:8080
2026/03/04 12:00:00 INFO polling repository=owner/your-project interval=30s
2026/03/04 12:00:30 INFO issue discovered number=1 title="Add a README"
2026/03/04 12:00:30 INFO marking in-progress number=1
2026/03/04 12:00:30 INFO agent dispatched number=1 worktree=/tmp/symphony-workspaces/owner-your-project-1
```

Symphony polls GitHub every 30 seconds by default. When it finds issue #1 with the `symphony:todo` label it:

1. Relabels the issue to `symphony:in-progress`
2. Creates an isolated git worktree
3. Launches `claude --print` with the rendered prompt from `WORKFLOW.md`

---

## Step 7: Watch the dashboard

Open your browser to `http://localhost:8080`.

You will see a table of running sessions with one row for issue #1:

| Issue | Title | State | Started |
|-------|-------|-------|---------|
| #1 | Add a README | in-progress | 12:00:30 |

The table updates automatically. As the agent works, the state column reflects progress. When the agent finishes, the row shows `done` and a link to the draft PR appears.

---

## Step 8: What happens next

While the dashboard shows `in-progress`, the agent is:

1. **Working in an isolated worktree** at `workspace.root/<repo>-<issue-number>/` — your main checkout is untouched.
2. **Running `claude --print`** with your rendered prompt as input. The agent reads the issue title and description, writes code, and runs any commands it needs.
3. **Creating a draft PR** — when done, the agent runs `gh pr create --draft` which opens a pull request against your base branch.

Once the PR is created, symphony relabels the issue `symphony:done` and the worktree is cleaned up.

Review the draft PR, request changes or approve, and merge when ready.

---

## Step 9: Stopping symphony

Press **Ctrl+C** for a graceful shutdown:

```
^C
2026/03/04 12:05:00 INFO shutdown signal received
2026/03/04 12:05:00 INFO waiting for active agents to finish agents=1
2026/03/04 12:05:45 INFO all agents finished
2026/03/04 12:05:45 INFO goodbye
```

Symphony waits for any in-flight agents to complete before exiting. If you need to force-quit immediately, press Ctrl+C a second time.

---

## Next steps

- **Add more issues** — label them `symphony:todo` and symphony will pick them up on the next poll.
- **Set priorities** — add a `P0`, `P1`, or `P2` label alongside `symphony:todo` to control order. Lower number = higher priority.
- **Customize the prompt** — the body of `WORKFLOW.md` is a Go template. Add project conventions, testing requirements, or style guides to steer the agent.
- **Read the full workflow reference** — see `examples/workflow-full.md` for all available configuration options and template variables.
