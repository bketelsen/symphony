package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bjk/symphony/internal/config"
	"github.com/bjk/symphony/internal/planner"
)

func runCreateIssues(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("create-issues", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workflowPath := fs.String("workflow", "./WORKFLOW.md", "path to WORKFLOW.md config")
	dryRun := fs.Bool("dry-run", false, "print issues without creating them")
	planRef := fs.String("plan-ref", "", "URL or path to full plan (linked in issue body)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: symphony create-issues [flags] <plan.md>")
	}
	planPath := fs.Arg(0)

	// Load config to get repo and label settings
	cfg, _, err := config.ParseWorkflowFile(*workflowPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Tracker.ActiveStates) == 0 {
		return fmt.Errorf("no active_states configured in %s", *workflowPath)
	}

	// Parse plan
	plan, err := planner.ParseFile(planPath)
	if err != nil {
		return fmt.Errorf("parse plan: %w", err)
	}

	if len(plan.Tasks) == 0 {
		fmt.Fprintln(stdout, "No tasks found in plan.")
		return nil
	}

	fmt.Fprintf(stdout, "Plan: %s (%d tasks)\n", plan.Title, len(plan.Tasks))
	fmt.Fprintf(stdout, "Repo: %s\n", cfg.Tracker.Repo)
	fmt.Fprintf(stdout, "Label: %s\n\n", cfg.Tracker.ActiveStates[0])

	// Create issues
	runner := &execCommandRunner{}
	creator := planner.NewIssueCreator(cfg.Tracker.Repo, cfg.Tracker.ActiveStates[0], runner)

	results, err := creator.CreateAll(context.Background(), plan, planner.CreateOptions{
		DryRun:  *dryRun,
		PlanRef: *planRef,
		Writer:  stdout,
	})
	if err != nil {
		// Print partial results before the error
		for _, r := range results {
			fmt.Fprintf(stdout, "Created #%d: %s (%s)\n", r.IssueNumber, r.Title, r.URL)
		}
		return err
	}

	if !*dryRun {
		for _, r := range results {
			fmt.Fprintf(stdout, "Created #%d: %s (%s)\n", r.IssueNumber, r.Title, r.URL)
		}
		fmt.Fprintf(stdout, "\n%d issues created.\n", len(results))
	}

	return nil
}
