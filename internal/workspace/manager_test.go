package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockExecutor struct {
	calls   []mockCall
	err     error
	handler func(dir, name string, args []string) ([]byte, error) // optional per-call handler
}

type mockCall struct {
	Dir  string
	Name string
	Args []string
}

func (m *mockExecutor) RunCommand(_ context.Context, dir string, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, mockCall{Dir: dir, Name: name, Args: args})
	if m.handler != nil {
		return m.handler(dir, name, args)
	}
	return nil, m.err
}

func TestSanitizeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"slashes and hash", "owner/repo#123", "owner_repo_123"},
		{"already clean", "my-workspace.v1", "my-workspace.v1"},
		{"spaces", "hello world", "hello_world"},
		{"empty", "", ""},
		{"special chars", "a@b!c$d", "a_b_c_d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeKey(tt.raw)
			if got != tt.want {
				t.Errorf("SanitizeKey(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestWorkspacePath(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp/ws", "git@github.com:o/r.git", "main", nil)

	path, err := m.WorkspacePath("owner_repo_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(path) != "owner_repo_1" {
		t.Errorf("path = %q, want basename owner_repo_1", path)
	}
}

func TestWorkspacePathEmptyKey(t *testing.T) {
	t.Parallel()

	m := NewManager("/tmp/ws", "git@github.com:o/r.git", "main", nil)
	_, err := m.WorkspacePath("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestWorkspacePathTraversal(t *testing.T) {
	t.Parallel()

	// SanitizeKey converts ".." to ".." (dots are allowed), but the path
	// check should still work since ".." is a valid key name after sanitization
	// that resolves to a parent directory.
	m := NewManager("/tmp/ws", "git@github.com:o/r.git", "main", nil)
	// Key with dots only becomes ".." which after sanitization is still ".."
	// and that's fine because filepath.Join("/tmp/ws", "..") = "/tmp"
	// which is outside root.
	_, err := m.WorkspacePath("..")
	if err == nil {
		t.Fatal("expected error for path traversal with '..'")
	}
}

func TestSetupBareClone(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	root := t.TempDir()
	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	if err := m.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	if len(exec.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(exec.calls))
	}
	call := exec.calls[0]
	if call.Name != "git" || call.Args[0] != "clone" || call.Args[1] != "--bare" {
		t.Errorf("unexpected command: %s %v", call.Name, call.Args)
	}
}

func TestSetupSkipsExistingBare(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{
		handler: func(dir, name string, args []string) ([]byte, error) {
			// Return the matching remote URL
			return []byte("git@github.com:o/r.git\n"), nil
		},
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".bare"), 0755)

	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	if err := m.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// Should call git config to verify remote, but not clone
	if len(exec.calls) != 1 {
		t.Fatalf("got %d calls, want 1 (remote check only)", len(exec.calls))
	}
	if exec.calls[0].Args[0] != "config" {
		t.Errorf("expected git config call, got %v", exec.calls[0].Args)
	}
}

func TestSetupRejectsMismatchedBare(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{
		handler: func(dir, name string, args []string) ([]byte, error) {
			// Return a different remote URL
			return []byte("git@github.com:other/repo.git\n"), nil
		},
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".bare"), 0755)

	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	err := m.Setup(context.Background())
	if err == nil {
		t.Fatal("expected error for mismatched bare clone, got nil")
	}
	if !strings.Contains(err.Error(), "other/repo.git") {
		t.Errorf("error should mention existing URL, got: %v", err)
	}
	if !strings.Contains(err.Error(), "o/r.git") {
		t.Errorf("error should mention expected URL, got: %v", err)
	}
}

func TestCreateWorktree(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".bare"), 0755)

	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	path, created, err := m.Create(context.Background(), "issue-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created {
		t.Error("expected created=true for new workspace")
	}
	if filepath.Base(path) != "issue-1" {
		t.Errorf("path basename = %q, want %q", filepath.Base(path), "issue-1")
	}

	// Verify git worktree add command
	if len(exec.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(exec.calls))
	}
	call := exec.calls[0]
	if call.Args[0] != "worktree" || call.Args[1] != "add" {
		t.Errorf("unexpected args: %v", call.Args)
	}
	if call.Args[3] != "symphony/issue-1" {
		t.Errorf("branch name = %q, want %q", call.Args[3], "symphony/issue-1")
	}
}

func TestCreateWorktreeReuse(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	root := t.TempDir()
	// Pre-create the workspace directory
	os.MkdirAll(filepath.Join(root, "issue-1"), 0755)

	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	path, created, err := m.Create(context.Background(), "issue-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created {
		t.Error("expected created=false for existing workspace")
	}
	if filepath.Base(path) != "issue-1" {
		t.Errorf("path basename = %q, want %q", filepath.Base(path), "issue-1")
	}
	if len(exec.calls) != 0 {
		t.Errorf("got %d calls, want 0 (should reuse)", len(exec.calls))
	}
}

func TestRemoveWorktree(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".bare"), 0755)

	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	if err := m.Remove(context.Background(), "issue-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if len(exec.calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(exec.calls))
	}
	// First call: git worktree remove
	if exec.calls[0].Args[0] != "worktree" || exec.calls[0].Args[1] != "remove" {
		t.Errorf("call[0] = %v, want worktree remove", exec.calls[0].Args)
	}
	// Second call: git branch -D
	if exec.calls[1].Args[0] != "branch" || exec.calls[1].Args[1] != "-D" {
		t.Errorf("call[1] = %v, want branch -D", exec.calls[1].Args)
	}
}

func TestCreateWorktreeError(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{err: fmt.Errorf("git error")}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".bare"), 0755)

	m := NewManager(root, "git@github.com:o/r.git", "main", exec)

	_, _, err := m.Create(context.Background(), "issue-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
