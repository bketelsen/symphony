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

func TestAddLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		issueNumber int
		label       string
		wantArgs    []string
	}{
		{
			name:        "adds label to issue",
			issueNumber: 42,
			label:       "symphony:in-progress",
			wantArgs:    []string{"issue", "edit", "42", "--repo", "owner/repo", "--add-label", "symphony:in-progress"},
		},
		{
			name:        "different issue and label",
			issueNumber: 7,
			label:       "symphony:done",
			wantArgs:    []string{"issue", "edit", "7", "--repo", "owner/repo", "--add-label", "symphony:done"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner := &mockRunner{}
			client := NewGitHubClient("owner/repo",
				[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

			err := client.AddLabel(context.Background(), tt.issueNumber, tt.label)
			if err != nil {
				t.Fatalf("AddLabel: %v", err)
			}

			if len(runner.calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(runner.calls))
			}
			got := runner.calls[0]
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("got args %v, want %v", got, tt.wantArgs)
			}
			for i, want := range tt.wantArgs {
				if got[i] != want {
					t.Errorf("args[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

func TestAddLabelError(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{err: fmt.Errorf("gh error")}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	err := client.AddLabel(context.Background(), 1, "label")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		issueNumber int
		label       string
		wantArgs    []string
	}{
		{
			name:        "removes label from issue",
			issueNumber: 42,
			label:       "symphony:todo",
			wantArgs:    []string{"issue", "edit", "42", "--repo", "owner/repo", "--remove-label", "symphony:todo"},
		},
		{
			name:        "different issue and label",
			issueNumber: 10,
			label:       "symphony:in-progress",
			wantArgs:    []string{"issue", "edit", "10", "--repo", "owner/repo", "--remove-label", "symphony:in-progress"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner := &mockRunner{}
			client := NewGitHubClient("owner/repo",
				[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

			err := client.RemoveLabel(context.Background(), tt.issueNumber, tt.label)
			if err != nil {
				t.Fatalf("RemoveLabel: %v", err)
			}

			if len(runner.calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(runner.calls))
			}
			got := runner.calls[0]
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("got args %v, want %v", got, tt.wantArgs)
			}
			for i, want := range tt.wantArgs {
				if got[i] != want {
					t.Errorf("args[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

func TestRemoveLabelError(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{err: fmt.Errorf("gh error")}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	err := client.RemoveLabel(context.Background(), 1, "label")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMarkPRReady(t *testing.T) {
	t.Parallel()

	prListJSON := `[
		{"number": 5, "headRefName": "symphony/repo-42-fix-bug"},
		{"number": 8, "headRefName": "feature/unrelated"},
		{"number": 12, "headRefName": "symphony/repo-10-add-tests"}
	]`

	tests := []struct {
		name        string
		issueNumber int
		prListJSON  string
		wantCalls   int
		wantReady   []string // args of the "pr ready" call, if any
	}{
		{
			name:        "finds matching PR and calls gh pr ready",
			issueNumber: 42,
			prListJSON:  prListJSON,
			wantCalls:   2, // pr list + pr ready
			wantReady:   []string{"pr", "ready", "5", "--repo", "owner/repo"},
		},
		{
			name:        "no matching PR returns no error",
			issueNumber: 999,
			prListJSON:  prListJSON,
			wantCalls:   1, // pr list only
		},
		{
			name:        "empty PR list returns no error",
			issueNumber: 42,
			prListJSON:  `[]`,
			wantCalls:   1, // pr list only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			callIdx := 0
			runner := &mockRunner{}
			// The runner returns prListJSON on the first call, empty on subsequent calls
			origRun := runner.Run
			_ = origRun

			// Use a custom runner that returns different output per call
			cr := &callTrackingRunner{
				outputs: [][]byte{[]byte(tt.prListJSON), {}},
			}

			client := NewGitHubClient("owner/repo",
				[]string{"symphony:todo"}, []string{"symphony:done"}, cr)

			err := client.MarkPRReady(context.Background(), tt.issueNumber)
			if err != nil {
				t.Fatalf("MarkPRReady: %v", err)
			}

			if len(cr.calls) != tt.wantCalls {
				t.Fatalf("got %d calls, want %d; calls: %v", len(cr.calls), tt.wantCalls, cr.calls)
			}

			if tt.wantReady != nil {
				got := cr.calls[1]
				if len(got) != len(tt.wantReady) {
					t.Fatalf("ready args = %v, want %v", got, tt.wantReady)
				}
				for i, want := range tt.wantReady {
					if got[i] != want {
						t.Errorf("ready args[%d] = %q, want %q", i, got[i], want)
					}
				}
			}
			_ = callIdx
		})
	}
}

func TestMarkPRReadyListError(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{err: fmt.Errorf("gh error")}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	err := client.MarkPRReady(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// callTrackingRunner returns pre-configured outputs for sequential calls.
type callTrackingRunner struct {
	outputs [][]byte
	calls   [][]string
}

func (r *callTrackingRunner) Run(_ context.Context, args []string) ([]byte, error) {
	idx := len(r.calls)
	r.calls = append(r.calls, args)
	if idx < len(r.outputs) {
		return r.outputs[idx], nil
	}
	return nil, nil
}

func TestGetPRStatus(t *testing.T) {
	t.Parallel()

	prListJSON := `[
		{"number": 5, "headRefName": "symphony/repo_42", "isDraft": true, "state": "OPEN"},
		{"number": 8, "headRefName": "feature/unrelated", "isDraft": false, "state": "OPEN"},
		{"number": 12, "headRefName": "symphony/repo_10", "isDraft": false, "state": "MERGED"}
	]`

	tests := []struct {
		name        string
		issueNumber int
		prListJSON  string
		wantFound   bool
		wantDraft   bool
		wantMerged  bool
		wantNumber  int
	}{
		{
			name:        "finds draft PR",
			issueNumber: 42,
			prListJSON:  prListJSON,
			wantFound:   true,
			wantDraft:   true,
			wantMerged:  false,
			wantNumber:  5,
		},
		{
			name:        "finds merged PR",
			issueNumber: 10,
			prListJSON:  prListJSON,
			wantFound:   true,
			wantDraft:   false,
			wantMerged:  true,
			wantNumber:  12,
		},
		{
			name:        "no matching PR",
			issueNumber: 999,
			prListJSON:  prListJSON,
			wantFound:   false,
		},
		{
			name:        "empty PR list",
			issueNumber: 42,
			prListJSON:  `[]`,
			wantFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := &mockRunner{output: []byte(tt.prListJSON)}
			client := NewGitHubClient("owner/repo",
				[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

			status, err := client.GetPRStatus(context.Background(), tt.issueNumber)
			if err != nil {
				t.Fatalf("GetPRStatus: %v", err)
			}

			if status.Found != tt.wantFound {
				t.Errorf("Found = %v, want %v", status.Found, tt.wantFound)
			}
			if tt.wantFound {
				if status.Number != tt.wantNumber {
					t.Errorf("Number = %d, want %d", status.Number, tt.wantNumber)
				}
				if status.IsDraft != tt.wantDraft {
					t.Errorf("IsDraft = %v, want %v", status.IsDraft, tt.wantDraft)
				}
				if status.Merged != tt.wantMerged {
					t.Errorf("Merged = %v, want %v", status.Merged, tt.wantMerged)
				}
			}

			// Verify correct gh args
			if len(runner.calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(runner.calls))
			}
			args := runner.calls[0]
			if args[0] != "pr" || args[1] != "list" {
				t.Errorf("unexpected command: %v", args[:2])
			}
			// Should use --state all to find merged PRs too
			foundStateAll := false
			for i, a := range args {
				if a == "--state" && i+1 < len(args) && args[i+1] == "all" {
					foundStateAll = true
				}
			}
			if !foundStateAll {
				t.Error("expected --state all in args")
			}
		})
	}
}

func TestGetPRStatusError(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{err: fmt.Errorf("gh error")}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	_, err := client.GetPRStatus(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCloseIssue(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	err := client.CloseIssue(context.Background(), 42)
	if err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(runner.calls))
	}
	wantArgs := []string{"issue", "close", "42", "--repo", "owner/repo"}
	got := runner.calls[0]
	if len(got) != len(wantArgs) {
		t.Fatalf("got args %v, want %v", got, wantArgs)
	}
	for i, want := range wantArgs {
		if got[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want)
		}
	}
}

func TestCloseIssueError(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{err: fmt.Errorf("gh error")}
	client := NewGitHubClient("owner/repo",
		[]string{"symphony:todo"}, []string{"symphony:done"}, runner)

	err := client.CloseIssue(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func intPtr(v int) *int { return &v }
