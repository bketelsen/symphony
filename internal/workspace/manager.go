package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Executor runs shell commands.
type Executor interface {
	RunCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// Manager manages git worktree workspaces.
type Manager struct {
	root       string
	repoURL    string
	baseBranch string
	executor   Executor
}

// NewManager creates a workspace manager.
func NewManager(root, repoURL, baseBranch string, executor Executor) *Manager {
	return &Manager{
		root:       root,
		repoURL:    repoURL,
		baseBranch: baseBranch,
		executor:   executor,
	}
}

var sanitizePattern = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// SanitizeKey ensures a workspace key contains only safe characters.
func SanitizeKey(raw string) string {
	return sanitizePattern.ReplaceAllString(raw, "_")
}

// WorkspacePath returns the absolute path for a workspace key.
// Returns an error if the key would escape the root directory.
func (m *Manager) WorkspacePath(key string) (string, error) {
	safe := SanitizeKey(key)
	if safe == "" {
		return "", fmt.Errorf("workspace: empty key after sanitization")
	}

	path := filepath.Join(m.root, safe)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve path: %w", err)
	}

	rootAbs, err := filepath.Abs(m.root)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve root: %w", err)
	}

	// Prevent path traversal
	if !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) && abs != rootAbs {
		return "", fmt.Errorf("workspace: path %q escapes root %q", abs, rootAbs)
	}

	return abs, nil
}

// Setup clones a bare repo at <root>/.bare if not already present.
func (m *Manager) Setup(ctx context.Context) error {
	bareDir := filepath.Join(m.root, ".bare")

	if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
		return nil // already set up
	}

	if err := os.MkdirAll(m.root, 0755); err != nil {
		return fmt.Errorf("workspace: create root: %w", err)
	}

	_, err := m.executor.RunCommand(ctx, m.root,
		"git", "clone", "--bare", m.repoURL, ".bare")
	if err != nil {
		return fmt.Errorf("workspace: bare clone: %w", err)
	}

	return nil
}

// Create creates or reuses a worktree for the given key.
// Returns (path, createdNow, error).
func (m *Manager) Create(ctx context.Context, key string) (string, bool, error) {
	wsPath, err := m.WorkspacePath(key)
	if err != nil {
		return "", false, err
	}

	// Check if workspace already exists
	if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
		return wsPath, false, nil
	}

	bareDir := filepath.Join(m.root, ".bare")
	branchName := "symphony/" + SanitizeKey(key)

	_, err = m.executor.RunCommand(ctx, bareDir,
		"git", "worktree", "add", "-b", branchName, wsPath, m.baseBranch)
	if err != nil {
		return "", false, fmt.Errorf("workspace: create worktree: %w", err)
	}

	return wsPath, true, nil
}

// Remove removes a worktree and its branch.
func (m *Manager) Remove(ctx context.Context, key string) error {
	wsPath, err := m.WorkspacePath(key)
	if err != nil {
		return err
	}

	bareDir := filepath.Join(m.root, ".bare")
	branchName := "symphony/" + SanitizeKey(key)

	if _, err := m.executor.RunCommand(ctx, bareDir,
		"git", "worktree", "remove", wsPath); err != nil {
		return fmt.Errorf("workspace: remove worktree: %w", err)
	}

	if _, err := m.executor.RunCommand(ctx, bareDir,
		"git", "branch", "-D", branchName); err != nil {
		return fmt.Errorf("workspace: delete branch: %w", err)
	}

	return nil
}
