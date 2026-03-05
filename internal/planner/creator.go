package planner

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/bjk/symphony/internal/tracker"
)

// IssueCreator creates GitHub issues from a parsed plan.
type IssueCreator struct {
	repo      string
	todoLabel string
	runner    tracker.CommandRunner
}

// NewIssueCreator creates an IssueCreator.
func NewIssueCreator(repo, todoLabel string, runner tracker.CommandRunner) *IssueCreator {
	return &IssueCreator{
		repo:      repo,
		todoLabel: todoLabel,
		runner:    runner,
	}
}

// CreateOptions configures issue creation behavior.
type CreateOptions struct {
	DryRun  bool
	PlanRef string    // URL or path to full plan, linked in issue body
	Writer  io.Writer // for dry-run and summary output
}

// CreateResult holds the result of creating a single issue.
type CreateResult struct {
	TaskNumber  int
	IssueNumber int
	Title       string
	URL         string
}

// CreateAll creates GitHub issues for all tasks in the plan.
// Issues are created in order so dependency references can be resolved.
func (c *IssueCreator) CreateAll(ctx context.Context, plan *Plan, opts CreateOptions) ([]CreateResult, error) {
	taskMap := make(map[int]int) // plan task number → GitHub issue number
	var results []CreateResult

	for _, task := range plan.Tasks {
		body := c.buildBody(task, taskMap, opts.PlanRef)
		labels := c.buildLabels(task)

		if opts.DryRun {
			fmt.Fprintf(opts.Writer, "[DRY RUN] Task %d: %s\n", task.Number, task.Title)
			fmt.Fprintf(opts.Writer, "  Labels: %v\n", labels)
			if len(task.DependsOn) > 0 {
				var deps []string
				for _, d := range task.DependsOn {
					deps = append(deps, fmt.Sprintf("Task %d → #%d", d, taskMap[d]))
				}
				fmt.Fprintf(opts.Writer, "  Dependencies: %s\n", strings.Join(deps, ", "))
			}
			fmt.Fprintf(opts.Writer, "\n")
			taskMap[task.Number] = 9000 + task.Number
			results = append(results, CreateResult{
				TaskNumber:  task.Number,
				IssueNumber: 9000 + task.Number,
				Title:       task.Title,
			})
			continue
		}

		issueNumber, url, err := c.createIssue(ctx, task.Title, body, labels)
		if err != nil {
			return results, fmt.Errorf("create issue for Task %d (%s): %w", task.Number, task.Title, err)
		}

		taskMap[task.Number] = issueNumber
		results = append(results, CreateResult{
			TaskNumber:  task.Number,
			IssueNumber: issueNumber,
			Title:       task.Title,
			URL:         url,
		})
	}

	return results, nil
}

func (c *IssueCreator) buildBody(task Task, taskMap map[int]int, planRef string) string {
	var sb strings.Builder

	sb.WriteString("## What to build\n\n")
	sb.WriteString(task.Body)
	sb.WriteString("\n\n")

	if len(task.FilesCreate) > 0 || len(task.FilesModify) > 0 {
		sb.WriteString("## Files\n\n")
		for _, f := range task.FilesCreate {
			sb.WriteString(fmt.Sprintf("- Create: `%s`\n", f))
		}
		for _, f := range task.FilesModify {
			sb.WriteString(fmt.Sprintf("- Modify: `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(task.DependsOn) > 0 {
		sb.WriteString("## Dependencies\n\n")
		for _, depTaskNum := range task.DependsOn {
			if issueNum, ok := taskMap[depTaskNum]; ok {
				sb.WriteString(fmt.Sprintf("Blocked by #%d\n", issueNum))
			}
		}
		sb.WriteString("\n")
	}

	if planRef != "" {
		sb.WriteString(fmt.Sprintf("---\n**Full plan:** %s\n", planRef))
	}

	return sb.String()
}

func (c *IssueCreator) buildLabels(task Task) []string {
	labels := []string{c.todoLabel}

	// Sequential priority: Task 1→P0, Task 2→P1, Task 3+→P2
	priority := task.Number - 1
	if priority > 2 {
		priority = 2
	}
	labels = append(labels, fmt.Sprintf("P%d", priority))

	return labels
}

var issueURLRe = regexp.MustCompile(`/issues/(\d+)`)

func (c *IssueCreator) createIssue(ctx context.Context, title, body string, labels []string) (int, string, error) {
	args := []string{
		"issue", "create",
		"--repo", c.repo,
		"--title", title,
		"--body", body,
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	out, err := c.runner.Run(ctx, args)
	if err != nil {
		return 0, "", fmt.Errorf("gh issue create: %w", err)
	}

	url := strings.TrimSpace(string(out))
	num, err := extractIssueNumberFromURL(url)
	if err != nil {
		return 0, url, err
	}

	return num, url, nil
}

func extractIssueNumberFromURL(url string) (int, error) {
	m := issueURLRe.FindStringSubmatch(url)
	if m == nil {
		return 0, fmt.Errorf("planner: cannot parse issue number from URL: %q", url)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("planner: invalid issue number in URL: %q", url)
	}
	return n, nil
}
