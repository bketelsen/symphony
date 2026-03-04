package tracker

import (
	"context"

	"github.com/bjk/symphony/internal/domain"
)

// TrackerClient fetches and queries issues from an issue tracker.
type TrackerClient interface {
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)
	FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]domain.Issue, error)
	FetchIssuesByStates(ctx context.Context, states []string) ([]domain.Issue, error)
}
