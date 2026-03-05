# Symphony `init` Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `symphony init` CLI subcommand that introspects a GitHub repo and generates a complete WORKFLOW.md with sensible defaults, plus creates required labels.

**Architecture:** New `cmd/symphony/init.go` with `runInit` function following the same pattern as `create_issues.go`. Uses `tracker.CommandRunner` (gh CLI) for GitHub introspection and label management. Subcommand routing added to `main.go`.

---

## Dependency Graph

```
Task 1 (init.go — introspection + WORKFLOW.md generation)
  └──► Task 2 (init_test.go — tests with mock runner)
         └──► Task 3 (main.go routing + build verification)
```

---

### Task 1: Init Command — Introspection and WORKFLOW.md Generation

**Files:**
- Create: `cmd/symphony/init.go`

**What to build:**

The `runInit` function that:
1. Parses flags (`--workspace-root`, `--force`)
2. Checks if WORKFLOW.md already exists (error unless `--force`)
3. Calls `gh repo view --json owner,name,sshUrl,defaultBranchRef` to introspect
4. Parses the JSON response to extract owner/name, sshUrl, defaultBranch
5. Generates WORKFLOW.md content from a template with introspected values
6. Writes WORKFLOW.md to disk
7. Calls `gh label list --repo <repo> --json name` to get existing labels
8. Creates missing labels via `gh label create --repo <repo> <name> --color <color>`
9. Prints summary to stdout

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bjk/symphony/internal/tracker"
)

// repoInfo holds introspected GitHub repo metadata.
type repoInfo struct {
	Owner         string
	Name          string
	SSHUrl        string
	DefaultBranch string
}

// labelDef defines a GitHub label to ensure exists.
type labelDef struct {
	Name  string
	Color string
}

var requiredLabels = []labelDef{
	{"symphony:todo", "0e8a16"},
	{"symphony:in-progress", "fbca04"},
	{"symphony:done", "0075ca"},
	{"symphony:cancelled", "cccccc"},
	{"P0", "d93f0b"},
	{"P1", "fbca04"},
	{"P2", "0075ca"},
}

func runInit(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceRoot := fs.String("workspace-root", "/tmp/symphony_workspaces", "workspace root directory")
	force := fs.Bool("force", false, "overwrite existing WORKFLOW.md")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check if WORKFLOW.md already exists
	if !*force {
		if _, err := os.Stat("WORKFLOW.md"); err == nil {
			return fmt.Errorf("WORKFLOW.md already exists (use --force to overwrite)")
		}
	}

	runner := &execCommandRunner{}
	ctx := context.Background()

	// Introspect repo
	info, err := introspectRepo(ctx, runner)
	if err != nil {
		return err
	}

	repo := info.Owner + "/" + info.Name
	fmt.Fprintf(stdout, "Detected repo: %s\n", repo)
	fmt.Fprintf(stdout, "Default branch: %s\n", info.DefaultBranch)
	fmt.Fprintf(stdout, "Clone URL: %s\n\n", info.SSHUrl)

	// Generate and write WORKFLOW.md
	content := generateWorkflow(repo, info.SSHUrl, info.DefaultBranch, *workspaceRoot)
	if err := os.WriteFile("WORKFLOW.md", []byte(content), 0644); err != nil {
		return fmt.Errorf("write WORKFLOW.md: %w", err)
	}
	fmt.Fprintln(stdout, "Created WORKFLOW.md")

	// Ensure labels exist
	if err := ensureLabels(ctx, runner, repo, stdout); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\nReady! Edit WORKFLOW.md to customize, then run: symphony\n")
	return nil
}

func introspectRepo(ctx context.Context, runner tracker.CommandRunner) (*repoInfo, error) {
	out, err := runner.Run(ctx, []string{
		"repo", "view", "--json", "owner,name,sshUrl,defaultBranchRef",
	})
	if err != nil {
		return nil, fmt.Errorf("introspect repo (is this a GitHub repo?): %w", err)
	}

	var raw struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name             string `json:"name"`
		SSHUrl           string `json:"sshUrl"`
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse repo info: %w", err)
	}

	branch := raw.DefaultBranchRef.Name
	if branch == "" {
		branch = "main"
	}

	return &repoInfo{
		Owner:         raw.Owner.Login,
		Name:          raw.Name,
		SSHUrl:        raw.SSHUrl,
		DefaultBranch: branch,
	}, nil
}

func generateWorkflow(repo, sshURL, branch, workspaceRoot string) string {
	// Use raw string concatenation to avoid template escaping issues
	// with Go template delimiters in the prompt section
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("tracker:\n")
	b.WriteString("  kind: github\n")
	b.WriteString(fmt.Sprintf("  repo: %s\n", repo))
	b.WriteString("  active_states:\n")
	b.WriteString("    - \"symphony:todo\"\n")
	b.WriteString("    - \"symphony:in-progress\"\n")
	b.WriteString("  terminal_states:\n")
	b.WriteString("    - \"symphony:done\"\n")
	b.WriteString("    - \"symphony:cancelled\"\n")
	b.WriteString("\n")
	b.WriteString("polling:\n")
	b.WriteString("  interval_ms: 30000\n")
	b.WriteString("\n")
	b.WriteString("workspace:\n")
	b.WriteString(fmt.Sprintf("  root: %s\n", workspaceRoot))
	b.WriteString(fmt.Sprintf("  repo_url: %s\n", sshURL))
	b.WriteString(fmt.Sprintf("  base_branch: %s\n", branch))
	b.WriteString("\n")
	b.WriteString("hooks:\n")
	b.WriteString("  before_run: |\n")
	b.WriteString(fmt.Sprintf("    git pull origin %s --rebase\n", branch))
	b.WriteString("  timeout_ms: 60000\n")
	b.WriteString("\n")
	b.WriteString("agent:\n")
	b.WriteString("  max_concurrent_agents: 2\n")
	b.WriteString("  max_turns: 10\n")
	b.WriteString("  max_retry_backoff_ms: 300000\n")
	b.WriteString("\n")
	b.WriteString("claude:\n")
	b.WriteString("  command: claude --print\n")
	b.WriteString("  model: sonnet\n")
	b.WriteString("  turn_timeout_ms: 3600000\n")
	b.WriteString("  stall_timeout_ms: 300000\n")
	b.WriteString("  permission_mode: bypassPermissions\n")
	b.WriteString("\n")
	b.WriteString("server:\n")
	b.WriteString("  port: 8080\n")
	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString("You are working on issue {{ .Issue.Identifier }}: {{ .Issue.Title }}\n")
	b.WriteString("\n")
	b.WriteString("{{ if .Issue.Description }}{{ deref .Issue.Description }}{{ end }}\n")
	b.WriteString("\n")
	b.WriteString("When creating pull requests, always create them as drafts using `gh pr create --draft`.\n")
	b.WriteString("\n")
	b.WriteString("{{ if gt .Attempt 0 }}\n")
	b.WriteString("This is continuation attempt {{ .Attempt }}.\n")
	b.WriteString("Check the current state and continue where the previous session left off.\n")
	b.WriteString("{{ end }}\n")
	return b.String()
}

func ensureLabels(ctx context.Context, runner tracker.CommandRunner, repo string, w *os.File) error {
	// Fetch existing labels
	out, err := runner.Run(ctx, []string{
		"label", "list", "--repo", repo, "--json", "name", "--limit", "100",
	})
	if err != nil {
		return fmt.Errorf("list labels: %w", err)
	}

	var existing []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &existing); err != nil {
		return fmt.Errorf("parse labels: %w", err)
	}

	existingSet := make(map[string]bool)
	for _, l := range existing {
		existingSet[l.Name] = true
	}

	// Create missing labels
	for _, label := range requiredLabels {
		if existingSet[label.Name] {
			fmt.Fprintf(w, "Label exists: %s (skipped)\n", label.Name)
			continue
		}
		_, err := runner.Run(ctx, []string{
			"label", "create", label.Name,
			"--repo", repo,
			"--color", label.Color,
		})
		if err != nil {
			return fmt.Errorf("create label %s: %w", label.Name, err)
		}
		fmt.Fprintf(w, "Created label: %s\n", label.Name)
	}

	return nil
}
```

**Depends on:** nothing

**Commit:** `feat: add symphony init command`

---

### Task 2: Init Command Tests

**Files:**
- Create: `cmd/symphony/init_test.go`

**What to build:**

Tests using a mock `CommandRunner` to verify:

1. **Happy path** — introspects repo, generates correct WORKFLOW.md content, creates missing labels
2. **Existing WORKFLOW.md without --force** — returns error
3. **Existing WORKFLOW.md with --force** — overwrites successfully
4. **gh repo view fails** — returns descriptive error
5. **Label already exists** — skips with "skipped" message
6. **Custom workspace root** — appears in generated content
7. **Default branch fallback** — empty `defaultBranchRef.name` → uses "main"

The test needs a `mockRunner` that can return different outputs for different commands. Use a callback-based mock:

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockInitRunner struct {
	handler func(args []string) ([]byte, error)
	calls   [][]string
}

func (m *mockInitRunner) Run(_ context.Context, args []string) ([]byte, error) {
	m.calls = append(m.calls, args)
	return m.handler(args)
}

func TestRunInit_HappyPath(t *testing.T) {
	// Setup temp dir, cd into it
	// Mock gh repo view → returns repo JSON
	// Mock gh label list → returns empty []
	// Mock gh label create → returns success for each
	// Verify WORKFLOW.md written with correct content
	// Verify all 7 labels created
}

func TestRunInit_WorkflowExists(t *testing.T) {
	// Create WORKFLOW.md in temp dir
	// Run without --force → expect error containing "already exists"
}

func TestRunInit_WorkflowExistsForce(t *testing.T) {
	// Create WORKFLOW.md in temp dir
	// Run with --force → expect success, file overwritten
}

func TestRunInit_GhFails(t *testing.T) {
	// Mock gh repo view → returns error
	// Expect error containing "introspect repo"
}

func TestRunInit_LabelsExist(t *testing.T) {
	// Mock gh label list → returns all 7 labels
	// Expect no gh label create calls
	// Output contains "skipped" for each
}

func TestRunInit_CustomWorkspaceRoot(t *testing.T) {
	// Run with --workspace-root /custom/path
	// Verify WORKFLOW.md contains "root: /custom/path"
}

func TestRunInit_DefaultBranchFallback(t *testing.T) {
	// Mock gh repo view with empty defaultBranchRef.name
	// Verify WORKFLOW.md contains "base_branch: main"
}
```

The tests need to `os.Chdir` to a temp directory (and restore afterward) since `runInit` writes WORKFLOW.md to the current directory. Use `t.TempDir()` and `t.Chdir()` (Go 1.24+) or manual chdir/restore.

**Important:** The `runInit` function currently uses `execCommandRunner` directly. To make it testable, refactor to accept a `tracker.CommandRunner` parameter. Either:
- Add a package-level variable that tests can override, or
- Refactor `runInit` to take a runner parameter, with `main.go` passing `&execCommandRunner{}`

The cleaner approach is to extract the core logic into a function that takes a runner:

```go
func initWorkflow(ctx context.Context, runner tracker.CommandRunner, opts initOptions, stdout *os.File) error {
    // ... all the logic
}

func runInit(args []string, stdout, stderr *os.File) error {
    // Parse flags
    // Call initWorkflow with &execCommandRunner{}
}
```

**Depends on:** Task 1

**Commit:** `test: add init command tests`

---

### Task 3: Subcommand Routing and Build Verification

**Files:**
- Modify: `cmd/symphony/main.go:38-41` — add `case "init"` to subcommand switch

**What to build:**

Add the `init` case to the existing subcommand routing in `main.go`:

```go
switch args[0] {
case "create-issues":
    return runCreateIssues(args[1:], stdout, stderr)
case "init":
    return runInit(args[1:], stdout, stderr)
}
```

**Step 1:** Add the routing case to `cmd/symphony/main.go`

**Step 2:** Run `make check` to verify all tests pass (including new init tests)

**Step 3:** Run `make build` to verify binary builds

**Step 4:** Verify the binary responds to `symphony init --help` (should show flag usage from the FlagSet)

**Depends on:** Task 1, Task 2

**Commit:** `feat: add init subcommand routing`
