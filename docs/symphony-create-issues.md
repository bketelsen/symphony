# symphony-create-issues

## Overview

The `symphony-create-issues` skill is a Claude Code skill that converts markdown implementation plans into GitHub Issues labeled for Symphony orchestration. It bridges the gap between planning (in Claude Code) and execution (by Symphony agents).

Each task in the plan becomes a GitHub Issue with the appropriate `symphony:todo` label, a priority label, and dependency references — ready for the Symphony orchestrator to discover and dispatch.

## When It Triggers

- After the `writing-plans` skill completes an implementation plan
- When the user explicitly asks to create symphony issues from a plan
- When invoked via `/symphony-create-issues`

## Prerequisites

- `gh` CLI installed and authenticated:
  ```bash
  gh auth status
  ```
- Repository has symphony labels. Run `symphony init` or follow `examples/github-setup.md` to create them.
- A `WORKFLOW.md` in the current directory (or specify with `--workflow`).

## CLI Usage

```
symphony create-issues [flags] <plan.md>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--workflow PATH` | `./WORKFLOW.md` | Path to WORKFLOW.md |
| `--dry-run` | false | Print what would be created without creating issues |
| `--plan-ref URL` | — | URL or path to the full plan, linked in each issue body |

## Input Format

`symphony create-issues` expects a markdown plan with the following structure:

```markdown
# Plan Title

**Goal:** One sentence describing what this builds.

**Architecture:** 2-3 sentences about the approach.

---

### Task 1: Component Name

**Files:**
- Create: `path/to/new/file.go`
- Modify: `path/to/existing/file.go`

**What to build:**

Description of what needs to be implemented.

**Commit:** `feat: add component name`

---

### Task 2: Another Component

**Files:**
- Create: `path/to/another/file.go`

**Depends on:** Task 1

**What to build:**

Description of what needs to be implemented.

**Commit:** `feat: add another component`
```

Key elements:

- **H1 title** — `# Plan Title`
- **Goal and Architecture** — `**Goal:**` and `**Architecture:**` lines after the title
- **Task headings** — `### Task N: Title` for each task
- **Files section** — `**Files:**` with `- Create:` and `- Modify:` lines
- **Dependencies** — `**Depends on:** Task N` (optional)
- **Commit message** — `**Commit:**` line

See `internal/planner/testdata/sample_plan.md` for a complete example.

## What It Creates

For each task in the plan, `symphony create-issues` creates a GitHub Issue:

- **Title:** The task title (e.g., `Domain Types`)
- **Body:**
  - "What to build" section with the task description
  - Files list (create and modify)
  - Dependencies listed as `Blocked by #N` referencing earlier issue numbers
  - Optional link to the full plan (when `--plan-ref` is provided)
- **Labels:**
  - `symphony:todo` — marks the issue for orchestrator pickup
  - Priority label: `P0` for Task 1, `P1` for Task 2, `P2` for Task 3 and beyond
- **Ordering:** Tasks are created in plan order, so earlier tasks get lower issue numbers. Later tasks reference earlier ones by number in their `Blocked by` lines.

## Example

Given a 3-task plan, running with `--dry-run`:

```
symphony create-issues --dry-run docs/plans/2026-03-04-widget-system.md
```

Output:

```
[DRY RUN] Task 1: Domain Types
  Labels: [symphony:todo P0]

[DRY RUN] Task 2: Widget Service
  Labels: [symphony:todo P1]
  Dependencies: Task 1 → #9001

[DRY RUN] Task 3: HTTP Handlers
  Labels: [symphony:todo P2]
  Dependencies: Task 1 → #9001, Task 2 → #9002
```

Without `--dry-run`, the issues are created in your repository and the output shows the created issue URLs:

```
Created #9001: Domain Types
Created #9002: Widget Service (blocked by #9001)
Created #9003: HTTP Handlers (blocked by #9001, #9002)
```

## How Symphony Picks Them Up

Once issues are created with the `symphony:todo` label, the Symphony orchestrator discovers them on its next poll cycle:

1. **Discovery** — the orchestrator queries for open issues labeled `symphony:todo`
2. **Prioritization** — issues are sorted by priority label (`P0` first, then `P1`, `P2`, etc.)
3. **Dispatch** — the orchestrator assigns available agents to unblocked issues
4. **Blocking** — issues with unresolved `Blocked by #N` references wait until those issues are closed before being dispatched

This means you can create all issues at once from the plan, and Symphony handles the sequencing automatically based on the dependency graph.
