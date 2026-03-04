package domain

import (
	"testing"
	"time"
)

func intPtr(v int) *int       { return &v }
func timePtr(t time.Time) *time.Time { return &t }

func TestIsBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blockers  []BlockerRef
		completed map[string]struct{}
		want      bool
	}{
		{
			name:      "no blockers",
			blockers:  nil,
			completed: map[string]struct{}{},
			want:      false,
		},
		{
			name:      "blocker completed",
			blockers:  []BlockerRef{{ID: "id1", Identifier: "repo#1"}},
			completed: map[string]struct{}{"id1": {}},
			want:      false,
		},
		{
			name:      "blocker not completed",
			blockers:  []BlockerRef{{ID: "id1", Identifier: "repo#1"}},
			completed: map[string]struct{}{},
			want:      true,
		},
		{
			name: "one of two blockers completed",
			blockers: []BlockerRef{
				{ID: "id1", Identifier: "repo#1"},
				{ID: "id2", Identifier: "repo#2"},
			},
			completed: map[string]struct{}{"id1": {}},
			want:      true,
		},
		{
			name: "all blockers completed",
			blockers: []BlockerRef{
				{ID: "id1", Identifier: "repo#1"},
				{ID: "id2", Identifier: "repo#2"},
			},
			completed: map[string]struct{}{"id1": {}, "id2": {}},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			issue := Issue{BlockedBy: tt.blockers}
			got := issue.IsBlocked(tt.completed)
			if got != tt.want {
				t.Errorf("IsBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortCandidates(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		issues  []Issue
		wantIDs []string
	}{
		{
			name: "sort by priority ascending",
			issues: []Issue{
				{Identifier: "a", Priority: intPtr(3)},
				{Identifier: "b", Priority: intPtr(1)},
				{Identifier: "c", Priority: intPtr(2)},
			},
			wantIDs: []string{"b", "c", "a"},
		},
		{
			name: "nil priority sorts last",
			issues: []Issue{
				{Identifier: "a", Priority: nil},
				{Identifier: "b", Priority: intPtr(1)},
			},
			wantIDs: []string{"b", "a"},
		},
		{
			name: "same priority sorts by created_at oldest first",
			issues: []Issue{
				{Identifier: "a", Priority: intPtr(1), CreatedAt: timePtr(t2)},
				{Identifier: "b", Priority: intPtr(1), CreatedAt: timePtr(t1)},
			},
			wantIDs: []string{"b", "a"},
		},
		{
			name: "same priority and time sorts by identifier",
			issues: []Issue{
				{Identifier: "z", Priority: intPtr(1), CreatedAt: timePtr(t1)},
				{Identifier: "a", Priority: intPtr(1), CreatedAt: timePtr(t1)},
			},
			wantIDs: []string{"a", "z"},
		},
		{
			name: "both nil priority sorts by identifier",
			issues: []Issue{
				{Identifier: "z"},
				{Identifier: "a"},
			},
			wantIDs: []string{"a", "z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			issues := make([]Issue, len(tt.issues))
			copy(issues, tt.issues)
			SortCandidates(issues)
			for i, want := range tt.wantIDs {
				if issues[i].Identifier != want {
					t.Errorf("position %d: got %s, want %s", i, issues[i].Identifier, want)
				}
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		attempt        int
		isContinuation bool
		maxBackoffMs   int
		want           time.Duration
	}{
		{
			name:           "continuation always 1s",
			attempt:        5,
			isContinuation: true,
			maxBackoffMs:   300000,
			want:           time.Second,
		},
		{
			name:           "failure attempt 1 = 10s",
			attempt:        1,
			isContinuation: false,
			maxBackoffMs:   300000,
			want:           10 * time.Second,
		},
		{
			name:           "failure attempt 2 = 20s",
			attempt:        2,
			isContinuation: false,
			maxBackoffMs:   300000,
			want:           20 * time.Second,
		},
		{
			name:           "failure attempt 3 = 40s",
			attempt:        3,
			isContinuation: false,
			maxBackoffMs:   300000,
			want:           40 * time.Second,
		},
		{
			name:           "capped at max backoff",
			attempt:        10,
			isContinuation: false,
			maxBackoffMs:   300000,
			want:           300 * time.Second,
		},
		{
			name:           "attempt 0 treated as 1",
			attempt:        0,
			isContinuation: false,
			maxBackoffMs:   300000,
			want:           10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateBackoff(tt.attempt, tt.isContinuation, tt.maxBackoffMs)
			if got != tt.want {
				t.Errorf("CalculateBackoff(%d, %v, %d) = %v, want %v",
					tt.attempt, tt.isContinuation, tt.maxBackoffMs, got, tt.want)
			}
		})
	}
}

func TestRetryEntryIsReady(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		due  time.Time
		want bool
	}{
		{"past due", now.Add(-time.Minute), true},
		{"exactly now", now, true},
		{"future", now.Add(time.Minute), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry := RetryEntry{DueAt: tt.due}
			if got := entry.IsReady(now); got != tt.want {
				t.Errorf("IsReady(%v) = %v, want %v", now, got, tt.want)
			}
		})
	}
}
