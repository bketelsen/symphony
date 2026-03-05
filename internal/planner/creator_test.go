package planner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockRunner struct {
	calls    [][]string
	returns  []mockReturn
	callIdx  int
}

type mockReturn struct {
	out []byte
	err error
}

func (m *mockRunner) Run(_ context.Context, args []string) ([]byte, error) {
	m.calls = append(m.calls, args)
	if m.callIdx < len(m.returns) {
		r := m.returns[m.callIdx]
		m.callIdx++
		return r.out, r.err
	}
	m.callIdx++
	return nil, nil
}

func TestCreateAll_ThreeTasks(t *testing.T) {
	runner := &mockRunner{
		returns: []mockReturn{
			{out: []byte("https://github.com/owner/repo/issues/10\n")},
			{out: []byte("https://github.com/owner/repo/issues/11\n")},
			{out: []byte("https://github.com/owner/repo/issues/12\n")},
		},
	}

	creator := NewIssueCreator("owner/repo", "symphony:todo", runner)
	plan := &Plan{
		Title: "Test Plan",
		Tasks: []Task{
			{Number: 1, Title: "First Task", FilesCreate: []string{"a.go"}},
			{Number: 2, Title: "Second Task", DependsOn: []int{1}},
			{Number: 3, Title: "Third Task", DependsOn: []int{1, 2}},
		},
	}

	results, err := creator.CreateAll(context.Background(), plan, CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// Verify issue numbers
	if results[0].IssueNumber != 10 {
		t.Errorf("result[0] issue = %d, want 10", results[0].IssueNumber)
	}
	if results[1].IssueNumber != 11 {
		t.Errorf("result[1] issue = %d, want 11", results[1].IssueNumber)
	}
	if results[2].IssueNumber != 12 {
		t.Errorf("result[2] issue = %d, want 12", results[2].IssueNumber)
	}

	// Verify 3 gh calls were made
	if len(runner.calls) != 3 {
		t.Fatalf("got %d gh calls, want 3", len(runner.calls))
	}

	// All calls should have symphony:todo label
	for i, call := range runner.calls {
		args := strings.Join(call, " ")
		if !strings.Contains(args, "--label symphony:todo") {
			t.Errorf("call %d missing --label symphony:todo: %v", i, call)
		}
	}

	// First task: P0 label
	args0 := strings.Join(runner.calls[0], " ")
	if !strings.Contains(args0, "--label P0") {
		t.Errorf("Task 1 missing P0 label: %v", runner.calls[0])
	}

	// Second task: P1 label
	args1 := strings.Join(runner.calls[1], " ")
	if !strings.Contains(args1, "--label P1") {
		t.Errorf("Task 2 missing P1 label: %v", runner.calls[1])
	}

	// Third task: P2 label, body should contain "Blocked by #10" and "Blocked by #11"
	args2 := strings.Join(runner.calls[2], " ")
	if !strings.Contains(args2, "--label P2") {
		t.Errorf("Task 3 missing P2 label: %v", runner.calls[2])
	}
	if !strings.Contains(args2, "Blocked by #10") {
		t.Errorf("Task 3 body missing 'Blocked by #10': %v", runner.calls[2])
	}
	if !strings.Contains(args2, "Blocked by #11") {
		t.Errorf("Task 3 body missing 'Blocked by #11': %v", runner.calls[2])
	}
}

func TestCreateAll_DryRun(t *testing.T) {
	runner := &mockRunner{}
	creator := NewIssueCreator("owner/repo", "symphony:todo", runner)

	plan := &Plan{
		Tasks: []Task{
			{Number: 1, Title: "Task One"},
			{Number: 2, Title: "Task Two", DependsOn: []int{1}},
		},
	}

	var buf bytes.Buffer
	results, err := creator.CreateAll(context.Background(), plan, CreateOptions{
		DryRun: true,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}

	// No gh calls should be made
	if len(runner.calls) != 0 {
		t.Errorf("dry run made %d gh calls", len(runner.calls))
	}

	// Results should use fake issue numbers
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].IssueNumber != 9001 {
		t.Errorf("dry run issue number = %d, want 9001", results[0].IssueNumber)
	}

	// Output should contain task info
	output := buf.String()
	if !strings.Contains(output, "Task One") {
		t.Errorf("dry run output missing task name: %s", output)
	}
}

func TestCreateAll_ErrorMidCreation(t *testing.T) {
	runner := &mockRunner{
		returns: []mockReturn{
			{out: []byte("https://github.com/owner/repo/issues/10\n")},
			{err: fmt.Errorf("API rate limited")},
		},
	}

	creator := NewIssueCreator("owner/repo", "symphony:todo", runner)
	plan := &Plan{
		Tasks: []Task{
			{Number: 1, Title: "First"},
			{Number: 2, Title: "Second"},
			{Number: 3, Title: "Third"},
		},
	}

	results, err := creator.CreateAll(context.Background(), plan, CreateOptions{})
	if err == nil {
		t.Fatal("expected error")
	}

	// Should have partial results (just the first task)
	if len(results) != 1 {
		t.Errorf("got %d partial results, want 1", len(results))
	}
	if results[0].IssueNumber != 10 {
		t.Errorf("partial result issue = %d, want 10", results[0].IssueNumber)
	}
}

func TestCreateAll_PlanRef(t *testing.T) {
	runner := &mockRunner{
		returns: []mockReturn{
			{out: []byte("https://github.com/owner/repo/issues/1\n")},
		},
	}

	creator := NewIssueCreator("owner/repo", "symphony:todo", runner)
	plan := &Plan{
		Tasks: []Task{
			{Number: 1, Title: "Task"},
		},
	}

	_, err := creator.CreateAll(context.Background(), plan, CreateOptions{
		PlanRef: "https://github.com/owner/repo/blob/main/docs/plans/plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	args := strings.Join(runner.calls[0], " ")
	if !strings.Contains(args, "Full plan") {
		t.Errorf("body missing plan reference: %v", runner.calls[0])
	}
}

func TestExtractIssueNumberFromURL(t *testing.T) {
	tests := []struct {
		url     string
		want    int
		wantErr bool
	}{
		{"https://github.com/owner/repo/issues/42", 42, false},
		{"https://github.com/owner/repo/issues/1", 1, false},
		{"https://github.com/owner/repo/issues/123\n", 123, false},
		{"not a url", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := extractIssueNumberFromURL(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("extractIssueNumberFromURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("extractIssueNumberFromURL(%q) = %d, want %d", tt.url, got, tt.want)
		}
	}
}

func TestBuildLabels_Priority(t *testing.T) {
	creator := NewIssueCreator("owner/repo", "symphony:todo", nil)

	tests := []struct {
		taskNum  int
		wantPrio string
	}{
		{1, "P0"},
		{2, "P1"},
		{3, "P2"},
		{4, "P2"},
		{10, "P2"},
	}

	for _, tt := range tests {
		labels := creator.buildLabels(Task{Number: tt.taskNum})
		if labels[0] != "symphony:todo" {
			t.Errorf("Task %d: first label = %q, want symphony:todo", tt.taskNum, labels[0])
		}
		if labels[1] != tt.wantPrio {
			t.Errorf("Task %d: priority = %q, want %q", tt.taskNum, labels[1], tt.wantPrio)
		}
	}
}
