package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bjk/symphony/internal/tracker"
)

// repoInfo holds introspected repository metadata.
type repoInfo struct {
	Owner         string
	Name          string
	SSHUrl        string
	DefaultBranch string
}

// labelDef describes a label to ensure exists.
type labelDef struct {
	Name  string
	Color string
}

// requiredLabels are the labels symphony needs on the repository.
var requiredLabels = []labelDef{
	{Name: "symphony:todo", Color: "0e8a16"},
	{Name: "symphony:in-progress", Color: "fbca04"},
	{Name: "symphony:done", Color: "0075ca"},
	{Name: "symphony:cancelled", Color: "cccccc"},
	{Name: "P0", Color: "d93f0b"},
	{Name: "P1", Color: "fbca04"},
	{Name: "P2", Color: "0075ca"},
}

// initOptions holds parsed flags for the init command.
type initOptions struct {
	WorkspaceRoot string
	Force         bool
}

// runInit is the entry point for the "init" subcommand.
func runInit(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceRoot := fs.String("workspace-root", "", "root directory for worktree workspaces (default: /tmp/symphony_workspaces/<owner_repo>)")
	force := fs.Bool("force", false, "overwrite existing WORKFLOW.md")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := initOptions{
		WorkspaceRoot: *workspaceRoot,
		Force:         *force,
	}

	ctx := context.Background()
	runner := &execCommandRunner{}
	return initWorkflow(ctx, runner, opts, stdout)
}

// initWorkflow is the core logic for the init command, testable via mock runner.
func initWorkflow(ctx context.Context, runner tracker.CommandRunner, opts initOptions, w io.Writer) error {
	// Check if WORKFLOW.md already exists
	if _, err := os.Stat("WORKFLOW.md"); err == nil && !opts.Force {
		return fmt.Errorf("WORKFLOW.md already exists (use --force to overwrite)")
	}

	// Introspect the repository
	repo, err := introspectRepo(ctx, runner)
	if err != nil {
		return fmt.Errorf("introspect repo: %w", err)
	}

	repoSlug := repo.Owner + "/" + repo.Name
	fmt.Fprintf(w, "Repository: %s\n", repoSlug)
	fmt.Fprintf(w, "SSH URL:    %s\n", repo.SSHUrl)
	fmt.Fprintf(w, "Branch:     %s\n", repo.DefaultBranch)

	// Namespace workspace root by repo to prevent cross-repo collisions
	wsRoot := opts.WorkspaceRoot
	if wsRoot == "" {
		wsRoot = "/tmp/symphony_workspaces/" + strings.ReplaceAll(repoSlug, "/", "_")
	}

	// Generate and write WORKFLOW.md
	content := generateWorkflow(repoSlug, repo.SSHUrl, repo.DefaultBranch, wsRoot)
	if err := os.WriteFile("WORKFLOW.md", []byte(content), 0644); err != nil {
		return fmt.Errorf("write WORKFLOW.md: %w", err)
	}
	fmt.Fprintf(w, "\nWrote WORKFLOW.md\n")

	// Ensure labels exist
	if err := ensureLabels(ctx, runner, repoSlug, w); err != nil {
		return fmt.Errorf("ensure labels: %w", err)
	}

	fmt.Fprintf(w, "\nDone! Review WORKFLOW.md and run: symphony\n")
	return nil
}

// ghRepoViewResponse matches the JSON output of gh repo view.
type ghRepoViewResponse struct {
	Owner            struct{ Login string } `json:"owner"`
	Name             string                 `json:"name"`
	SSHURL           string                 `json:"sshUrl"`
	DefaultBranchRef struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

// introspectRepo calls gh repo view to get repository metadata.
func introspectRepo(ctx context.Context, runner tracker.CommandRunner) (*repoInfo, error) {
	out, err := runner.Run(ctx, []string{"repo", "view", "--json", "owner,name,sshUrl,defaultBranchRef"})
	if err != nil {
		return nil, fmt.Errorf("gh repo view: %w", err)
	}

	var resp ghRepoViewResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse gh repo view output: %w", err)
	}

	branch := resp.DefaultBranchRef.Name
	if branch == "" {
		branch = "main"
	}

	return &repoInfo{
		Owner:         resp.Owner.Login,
		Name:          resp.Name,
		SSHUrl:        resp.SSHURL,
		DefaultBranch: branch,
	}, nil
}

// generateWorkflow builds the WORKFLOW.md content with introspected values.
func generateWorkflow(repo, sshURL, branch, workspaceRoot string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("tracker:\n")
	b.WriteString("  kind: github\n")
	b.WriteString("  repo: " + repo + "\n")
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
	b.WriteString("  root: " + workspaceRoot + "\n")
	b.WriteString("  repo_url: " + sshURL + "\n")
	b.WriteString("  base_branch: " + branch + "\n")
	b.WriteString("\n")
	b.WriteString("hooks:\n")
	b.WriteString("  before_run: |\n")
	b.WriteString("    git pull origin " + branch + " --rebase\n")
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

// ghLabelEntry matches the JSON output of gh label list.
type ghLabelEntry struct {
	Name string `json:"name"`
}

// ensureLabels checks for missing labels and creates them.
func ensureLabels(ctx context.Context, runner tracker.CommandRunner, repo string, w io.Writer) error {
	// Get existing labels
	out, err := runner.Run(ctx, []string{"label", "list", "--repo", repo, "--json", "name", "--limit", "100"})
	if err != nil {
		return fmt.Errorf("gh label list: %w", err)
	}

	var existing []ghLabelEntry
	if err := json.Unmarshal(out, &existing); err != nil {
		return fmt.Errorf("parse gh label list output: %w", err)
	}

	existingSet := make(map[string]bool, len(existing))
	for _, l := range existing {
		existingSet[l.Name] = true
	}

	// Create missing labels
	for _, lbl := range requiredLabels {
		if existingSet[lbl.Name] {
			fmt.Fprintf(w, "Label exists: %s\n", lbl.Name)
			continue
		}
		_, err := runner.Run(ctx, []string{"label", "create", "--repo", repo, lbl.Name, "--color", lbl.Color})
		if err != nil {
			return fmt.Errorf("create label %s: %w", lbl.Name, err)
		}
		fmt.Fprintf(w, "Created label: %s\n", lbl.Name)
	}

	return nil
}
