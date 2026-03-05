package tracker

import (
	"context"

	"github.com/bjk/symphony/internal/domain"
)

// PRStatus describes the state of a pull request associated with an issue.
type PRStatus struct {
	Found   bool
	Number  int
	IsDraft bool
	Merged  bool
}

// TrackerClient fetches and queries issues from an issue tracker.
type TrackerClient interface {
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)
	FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]domain.Issue, error)
	FetchIssuesByStates(ctx context.Context, states []string) ([]domain.Issue, error)
	AddLabel(ctx context.Context, issueNumber int, label string) error
	RemoveLabel(ctx context.Context, issueNumber int, label string) error
	MarkPRReady(ctx context.Context, issueNumber int) error
	GetPRStatus(ctx context.Context, issueNumber int) (*PRStatus, error)
	CloseIssue(ctx context.Context, issueNumber int) error
}
