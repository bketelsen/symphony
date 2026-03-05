package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// mockInitRunner is a callback-based mock for tracker.CommandRunner.
type mockInitRunner struct {
	handler func(args []string) ([]byte, error)
	calls   [][]string
}

func (m *mockInitRunner) Run(_ context.Context, args []string) ([]byte, error) {
	m.calls = append(m.calls, args)
	return m.handler(args)
}

const validRepoJSON = `{"owner":{"login":"testowner"},"name":"testrepo","sshUrl":"git@github.com:testowner/testrepo.git","defaultBranchRef":{"name":"main"}}`

const allLabelsJSON = `[{"name":"symphony:todo"},{"name":"symphony:in-progress"},{"name":"symphony:done"},{"name":"symphony:cancelled"},{"name":"P0"},{"name":"P1"},{"name":"P2"}]`

// happyHandler returns valid responses for repo view and label operations.
func happyHandler(args []string) ([]byte, error) {
	switch args[0] {
	case "repo":
		return []byte(validRepoJSON), nil
	case "label":
		if len(args) > 1 && args[1] == "list" {
			return []byte("[]"), nil
		}
		// label create
		return []byte(""), nil
	}
	return nil, fmt.Errorf("unexpected command: %v", args)
}

func TestInitWorkflow_HappyPath(t *testing.T) {
	t.Chdir(t.TempDir())

	mock := &mockInitRunner{handler: happyHandler}
	var buf bytes.Buffer
	opts := initOptions{
		WorkspaceRoot: "/tmp/symphony_workspaces",
	}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify WORKFLOW.md written
	content, err := os.ReadFile("WORKFLOW.md")
	if err != nil {
		t.Fatalf("expected WORKFLOW.md to exist: %v", err)
	}
	s := string(content)
	for _, want := range []string{"testowner/testrepo", "git@github.com:testowner/testrepo.git", "base_branch: main"} {
		if !strings.Contains(s, want) {
			t.Errorf("WORKFLOW.md missing %q", want)
		}
	}

	// Verify 7 label create calls
	var createCalls int
	for _, c := range mock.calls {
		if len(c) >= 2 && c[0] == "label" && c[1] == "create" {
			createCalls++
		}
	}
	if createCalls != 7 {
		t.Errorf("expected 7 label create calls, got %d", createCalls)
	}

	// Verify output
	output := buf.String()
	for _, want := range []string{"Repository:", "Wrote WORKFLOW.md", "Created label:"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestInitWorkflow_WorkflowExists(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("WORKFLOW.md", []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	mock := &mockInitRunner{handler: happyHandler}
	var buf bytes.Buffer
	opts := initOptions{}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error containing 'already exists', got: %v", err)
	}
}

func TestInitWorkflow_WorkflowExistsForce(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("WORKFLOW.md", []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	mock := &mockInitRunner{handler: happyHandler}
	var buf bytes.Buffer
	opts := initOptions{
		WorkspaceRoot: "/tmp/symphony_workspaces",
		Force:         true,
	}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	content, err := os.ReadFile("WORKFLOW.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == "old content" {
		t.Error("WORKFLOW.md was not overwritten")
	}
}

func TestInitWorkflow_GhFails(t *testing.T) {
	t.Chdir(t.TempDir())

	mock := &mockInitRunner{
		handler: func(args []string) ([]byte, error) {
			return nil, fmt.Errorf("gh failed")
		},
	}
	var buf bytes.Buffer
	opts := initOptions{}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "introspect repo") {
		t.Errorf("expected error containing 'introspect repo', got: %v", err)
	}
}

func TestInitWorkflow_LabelsExist(t *testing.T) {
	t.Chdir(t.TempDir())

	mock := &mockInitRunner{
		handler: func(args []string) ([]byte, error) {
			switch args[0] {
			case "repo":
				return []byte(validRepoJSON), nil
			case "label":
				if len(args) > 1 && args[1] == "list" {
					return []byte(allLabelsJSON), nil
				}
				return []byte(""), nil
			}
			return nil, fmt.Errorf("unexpected: %v", args)
		},
	}
	var buf bytes.Buffer
	opts := initOptions{
		WorkspaceRoot: "/tmp/symphony_workspaces",
	}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify no label create calls
	for _, c := range mock.calls {
		if len(c) >= 2 && c[0] == "label" && c[1] == "create" {
			t.Errorf("unexpected label create call: %v", c)
		}
	}

	output := buf.String()
	if !strings.Contains(output, "Label exists:") {
		t.Errorf("output missing 'Label exists:', got: %s", output)
	}
}

func TestInitWorkflow_CustomWorkspaceRoot(t *testing.T) {
	t.Chdir(t.TempDir())

	mock := &mockInitRunner{handler: happyHandler}
	var buf bytes.Buffer
	opts := initOptions{
		WorkspaceRoot: "/custom/path",
	}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	content, err := os.ReadFile("WORKFLOW.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "root: /custom/path") {
		t.Error("WORKFLOW.md missing 'root: /custom/path'")
	}
}

func TestInitWorkflow_NamespacedWorkspaceRoot(t *testing.T) {
	t.Chdir(t.TempDir())

	mock := &mockInitRunner{handler: happyHandler}
	var buf bytes.Buffer
	opts := initOptions{
		WorkspaceRoot: "", // empty means auto-namespace
	}

	err := initWorkflow(context.Background(), mock, opts, &buf)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	content, err := os.ReadFile("WORKFLOW.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "root: /tmp/symphony_workspaces/testowner_testrepo") {
		t.Errorf("WORKFLOW.md should have namespaced workspace root, got:\n%s", content)
	}
}

func TestIntrospectRepo_DefaultBranchFallback(t *testing.T) {
	mock := &mockInitRunner{
		handler: func(args []string) ([]byte, error) {
			return []byte(`{"owner":{"login":"testowner"},"name":"testrepo","sshUrl":"git@github.com:testowner/testrepo.git","defaultBranchRef":{"name":""}}`), nil
		},
	}

	info, err := introspectRepo(context.Background(), mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if info.DefaultBranch != "main" {
		t.Errorf("expected DefaultBranch='main', got %q", info.DefaultBranch)
	}
}

func TestGenerateWorkflow(t *testing.T) {
	output := generateWorkflow("owner/repo", "git@github.com:owner/repo.git", "main", "/workspace")

	checks := []struct {
		desc string
		want string
	}{
		{"starts with front matter", "---\n"},
		{"contains repo", "repo: owner/repo"},
		{"contains repo_url", "repo_url: git@github.com:owner/repo.git"},
		{"contains base_branch", "base_branch: main"},
		{"contains issue identifier template", "{{ .Issue.Identifier }}"},
		{"ends with closing template", "{{ end }}\n"},
	}

	for _, tc := range checks {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.desc == "starts with front matter" {
				if !strings.HasPrefix(output, tc.want) {
					t.Errorf("expected output to start with %q", tc.want)
				}
			} else if tc.desc == "ends with closing template" {
				if !strings.HasSuffix(output, tc.want) {
					t.Errorf("expected output to end with %q, last 20 chars: %q", tc.want, output[len(output)-20:])
				}
			} else {
				if !strings.Contains(output, tc.want) {
					t.Errorf("expected output to contain %q", tc.want)
				}
			}
		})
	}
}
