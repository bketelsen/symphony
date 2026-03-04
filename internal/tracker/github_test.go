package tracker

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// mockRunner implements CommandRunner for testing.
type mockRunner struct {
	output []byte
	err    error
	calls  [][]string
}

func (m *mockRunner) Run(_ context.Context, args []string) ([]byte, error) {
	m.calls = append(m.calls, args)
	return m.output, m.err
}

func TestExtractPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels []string
		want   *int
	}{
		{"P2 label", []string{"bug", "P2"}, intPtr(2)},
		{"P0 label", []string{"P0"}, intPtr(0)},
		{"no priority", []string{"bug", "feature"}, nil},
		{"multiple P labels takes lowest", []string{"P3", "P1"}, intPtr(1)},
		{"P5 not valid", []string{"P5"}, nil},
		{"empty labels", []string{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractPriority(tt.labels)
			if tt.want == nil {
				if got != nil {
					t.Errorf("extractPriority(%v) = %d, want nil", tt.labels, *got)
				}
			} else {
				if got == nil {
					t.Errorf("extractPriority(%v) = nil, want %d", tt.labels, *tt.want)
				} else if *got != *tt.want {
					t.Errorf("extractPriority(%v) = %d, want %d", tt.labels, *got, *tt.want)
				}
			}
		})
	}
}

func TestExtractBlockers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want int // number of blockers
	}{
		{"single blocker", "blocked by #5", 1},
		{"multiple blockers", "blocked by #5 and blocked by #10", 2},
		{"case insensitive", "Blocked By #3", 1},
		{"no blockers", "This is a normal issue", 0},
		{"empty body", "", 0},
		{"blocker in context", "This is blocked by #7 because of deps", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractBlockers(tt.body, "owner/repo")
			if len(got) != tt.want {
				t.Errorf("extractBlockers(%q) returned %d refs, want %d", tt.body, len(got), tt.want)
			}
		})
	}
}

func TestExtractBlockersIdentifiers(t *testing.T) {
	t.Parallel()

	refs := extractBlockers("blocked by #42", "myorg/myrepo")
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].Identifier != "myorg/myrepo#42" {
		t.Errorf("identifier = %q, want %q", refs[0].Identifier, "myorg/myrepo#42")
	}
}

func TestMapStateFromLabels(t *testing.T) {
	t.Parallel()

	active := []string{"symphony:todo", "symphony:in-progress"}
	terminal := []string{"symphony:done", "symphony:cancelled"}

	tests := []struct {
		name    string
		labels  []string
		ghState string
		want    string
	}{
		{"active state label", []string{"symphony:todo"}, "open", "symphony:todo"},
		{"terminal state label", []string{"symphony:done"}, "open", "symphony:done"},
		{"terminal takes precedence over active", []string{"symphony:todo", "symphony:done"}, "open", "symphony:done"},
		{"closed with no label", []string{}, "closed", "symphony:done"},
		{"open with no matching label", []string{"bug"}, "open", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapStateFromLabels(tt.labels, tt.ghState, active, terminal)
			if got != tt.want {
				t.Errorf("mapStateFromLabels(%v, %q) = %q, want %q", tt.labels, tt.ghState, got, tt.want)
			}
		})
	}
}

func TestFetchCandidateIssues(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/issues.json")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	runner := &mockRunner{output: data}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	issues, err := client.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatalf("FetchCandidateIssues: %v", err)
	}

	if len(issues) != 3 {
		t.Fatalf("got %d issues, want 3", len(issues))
	}

	// Check first issue
	if issues[0].ID != "I_abc123" {
		t.Errorf("issues[0].ID = %q, want %q", issues[0].ID, "I_abc123")
	}
	if issues[0].Identifier != "owner/repo#1" {
		t.Errorf("issues[0].Identifier = %q, want %q", issues[0].Identifier, "owner/repo#1")
	}
	if issues[0].Priority == nil || *issues[0].Priority != 1 {
		t.Errorf("issues[0].Priority = %v, want 1", issues[0].Priority)
	}
	if len(issues[0].BlockedBy) != 1 {
		t.Errorf("issues[0].BlockedBy len = %d, want 1", len(issues[0].BlockedBy))
	}

	// Check no-priority issue
	if issues[2].Priority != nil {
		t.Errorf("issues[2].Priority = %v, want nil", issues[2].Priority)
	}

	// Check gh CLI args
	if len(runner.calls) != 1 {
		t.Fatalf("runner called %d times, want 1", len(runner.calls))
	}
	args := runner.calls[0]
	if args[0] != "issue" || args[1] != "list" {
		t.Errorf("unexpected command: %v", args[:2])
	}
}

func TestFetchCandidateIssuesError(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{err: fmt.Errorf("exit status 1")}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	_, err := client.FetchCandidateIssues(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchCandidateIssuesMalformedJSON(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{output: []byte(`not json`)}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	_, err := client.FetchCandidateIssues(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchCandidateIssuesEmptyResponse(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{output: []byte(`[]`)}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	issues, err := client.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("got %d issues, want 0", len(issues))
	}
}

func TestParseIssueNilDescription(t *testing.T) {
	t.Parallel()

	raw := ghIssue{
		ID:     "I_1",
		Number: 1,
		Title:  "Test",
		Body:   "",
	}
	issue := parseIssue(raw, "owner/repo", nil, nil)
	if issue.Description != nil {
		t.Errorf("Description = %v, want nil for empty body", issue.Description)
	}
}

func intPtr(v int) *int { return &v }
