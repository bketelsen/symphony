package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bjk/symphony/internal/domain"
)

// CommandRunner executes CLI commands and returns their output.
type CommandRunner interface {
	Run(ctx context.Context, args []string) ([]byte, error)
}

// GitHubClient implements TrackerClient using the gh CLI.
type GitHubClient struct {
	repo           string
	activeStates   []string
	terminalStates []string
	runner         CommandRunner
}

// NewGitHubClient creates a new GitHub Issues tracker client.
func NewGitHubClient(repo string, activeStates, terminalStates []string, runner CommandRunner) *GitHubClient {
	return &GitHubClient{
		repo:           repo,
		activeStates:   activeStates,
		terminalStates: terminalStates,
		runner:         runner,
	}
}

// ghIssue is the JSON shape returned by gh issue list --json.
type ghIssue struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (c *GitHubClient) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	args := []string{
		"issue", "list",
		"--repo", c.repo,
		"--state", "open",
		"--json", "id,number,title,body,state,url,createdAt,updatedAt,labels",
		"--limit", "100",
	}

	// Add label filters for active states
	for _, state := range c.activeStates {
		args = append(args, "--label", state)
	}

	out, err := c.runner.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("tracker: fetch candidates: %w", err)
	}

	return c.parseIssues(out)
}

func (c *GitHubClient) FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]domain.Issue, error) {
	// gh CLI doesn't support fetching by node ID directly, so we fetch all
	// open+closed issues and filter. For small repos this is acceptable;
	// for large repos a GraphQL query would be better.
	var result []domain.Issue

	for _, state := range []string{"open", "closed"} {
		args := []string{
			"issue", "list",
			"--repo", c.repo,
			"--state", state,
			"--json", "id,number,title,body,state,url,createdAt,updatedAt,labels",
			"--limit", "100",
		}

		out, err := c.runner.Run(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("tracker: fetch issues by IDs: %w", err)
		}

		issues, err := c.parseIssues(out)
		if err != nil {
			return nil, err
		}

		for _, issue := range issues {
			for _, id := range ids {
				if issue.ID == id {
					result = append(result, issue)
				}
			}
		}
	}

	return result, nil
}

func (c *GitHubClient) FetchIssuesByStates(ctx context.Context, states []string) ([]domain.Issue, error) {
	var result []domain.Issue

	for _, label := range states {
		args := []string{
			"issue", "list",
			"--repo", c.repo,
			"--state", "all",
			"--label", label,
			"--json", "id,number,title,body,state,url,createdAt,updatedAt,labels",
			"--limit", "100",
		}

		out, err := c.runner.Run(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("tracker: fetch issues by states: %w", err)
		}

		issues, err := c.parseIssues(out)
		if err != nil {
			return nil, err
		}
		result = append(result, issues...)
	}

	return result, nil
}

func (c *GitHubClient) parseIssues(data []byte) ([]domain.Issue, error) {
	var raw []ghIssue
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("tracker: parse response: %w", err)
	}

	issues := make([]domain.Issue, 0, len(raw))
	for _, r := range raw {
		issue := parseIssue(r, c.repo, c.activeStates, c.terminalStates)
		issues = append(issues, issue)
	}
	return issues, nil
}

func parseIssue(r ghIssue, repo string, activeStates, terminalStates []string) domain.Issue {
	labels := make([]string, len(r.Labels))
	for i, l := range r.Labels {
		labels[i] = l.Name
	}

	identifier := fmt.Sprintf("%s#%d", repo, r.Number)
	createdAt := r.CreatedAt
	updatedAt := r.UpdatedAt
	url := r.URL

	var desc *string
	if r.Body != "" {
		desc = &r.Body
	}

	return domain.Issue{
		ID:          r.ID,
		Identifier:  identifier,
		Number:      r.Number,
		Title:       r.Title,
		Description: desc,
		Priority:    extractPriority(labels),
		State:       mapStateFromLabels(labels, r.State, activeStates, terminalStates),
		URL:         &url,
		Labels:      labels,
		BlockedBy:   extractBlockers(r.Body, repo),
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}
}

// extractPriority reads P0-P4 labels and returns the lowest (highest priority).
func extractPriority(labels []string) *int {
	var best *int
	for _, l := range labels {
		if len(l) == 2 && l[0] == 'P' && l[1] >= '0' && l[1] <= '4' {
			p := int(l[1] - '0')
			if best == nil || p < *best {
				best = &p
			}
		}
	}
	return best
}

var blockerPattern = regexp.MustCompile(`(?i)blocked\s+by\s+#(\d+)`)

// extractBlockers parses "blocked by #N" patterns from issue body text.
func extractBlockers(body string, repo string) []domain.BlockerRef {
	matches := blockerPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	refs := make([]domain.BlockerRef, 0, len(matches))
	for _, m := range matches {
		num, _ := strconv.Atoi(m[1])
		refs = append(refs, domain.BlockerRef{
			Identifier: fmt.Sprintf("%s#%d", repo, num),
			ID:         fmt.Sprintf("%s#%d", repo, num), // use identifier as ID when we don't have node ID
		})
		_ = num
	}
	return refs
}

func (c *GitHubClient) AddLabel(ctx context.Context, issueNumber int, label string) error {
	args := []string{
		"issue", "edit", fmt.Sprintf("%d", issueNumber),
		"--repo", c.repo,
		"--add-label", label,
	}
	_, err := c.runner.Run(ctx, args)
	if err != nil {
		return fmt.Errorf("tracker: add label: %w", err)
	}
	return nil
}

func (c *GitHubClient) RemoveLabel(ctx context.Context, issueNumber int, label string) error {
	args := []string{
		"issue", "edit", fmt.Sprintf("%d", issueNumber),
		"--repo", c.repo,
		"--remove-label", label,
	}
	_, err := c.runner.Run(ctx, args)
	if err != nil {
		return fmt.Errorf("tracker: remove label: %w", err)
	}
	return nil
}

func (c *GitHubClient) MarkPRReady(ctx context.Context, issueNumber int) error {
	// Find PR by issue number — convention is branch name contains the issue number
	listArgs := []string{
		"pr", "list",
		"--repo", c.repo,
		"--json", "number,headRefName",
		"--limit", "100",
		"--state", "open",
	}
	out, err := c.runner.Run(ctx, listArgs)
	if err != nil {
		return fmt.Errorf("tracker: list PRs: %w", err)
	}

	var prs []struct {
		Number      int    `json:"number"`
		HeadRefName string `json:"headRefName"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return fmt.Errorf("tracker: parse PR list: %w", err)
	}

	// Find PR with branch matching symphony/<key> pattern containing issue number
	issueStr := fmt.Sprintf("%d", issueNumber)
	for _, pr := range prs {
		if strings.Contains(pr.HeadRefName, "symphony/") && strings.Contains(pr.HeadRefName, issueStr) {
			readyArgs := []string{
				"pr", "ready", fmt.Sprintf("%d", pr.Number),
				"--repo", c.repo,
			}
			_, err := c.runner.Run(ctx, readyArgs)
			if err != nil {
				return fmt.Errorf("tracker: mark PR ready: %w", err)
			}
			return nil
		}
	}

	return nil // no matching PR found — not an error
}

// mapStateFromLabels determines the symphony state from issue labels and GitHub state.
func mapStateFromLabels(labels []string, ghState string, activeStates, terminalStates []string) string {
	// Check terminal states first (higher precedence)
	for _, l := range labels {
		for _, ts := range terminalStates {
			if strings.EqualFold(l, ts) {
				return ts
			}
		}
	}

	// Check active states
	for _, l := range labels {
		for _, as := range activeStates {
			if strings.EqualFold(l, as) {
				return as
			}
		}
	}

	// Fallback to GitHub state
	if ghState == "closed" {
		if len(terminalStates) > 0 {
			return terminalStates[0]
		}
		return "closed"
	}

	return "unknown"
}
