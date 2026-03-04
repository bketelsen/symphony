package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const validWorkflow = `---
tracker:
  repo: owner/repo
  active_states:
    - "symphony:todo"

workspace:
  root: /tmp/ws
  repo_url: git@github.com:owner/repo.git
---

Prompt v1.
`

const updatedWorkflow = `---
tracker:
  repo: owner/repo
  active_states:
    - "symphony:todo"

polling:
  interval_ms: 5000

workspace:
  root: /tmp/ws
  repo_url: git@github.com:owner/repo.git
---

Prompt v2.
`

const invalidWorkflow = `---
tracker:
  repo: [[[broken
---

Bad.
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestConfigWatcherReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	writeFile(t, path, validWorkflow)

	var callCount atomic.Int32
	cw, err := NewConfigWatcher(path, func() {
		callCount.Add(1)
	})
	if err != nil {
		t.Fatalf("NewConfigWatcher: %v", err)
	}
	defer cw.Stop()

	// Verify initial parse
	cfg, prompt := cw.Current()
	if cfg.Polling.IntervalMs != 30000 {
		t.Errorf("initial interval = %d, want 30000", cfg.Polling.IntervalMs)
	}
	if prompt != "Prompt v1." {
		t.Errorf("initial prompt = %q, want %q", prompt, "Prompt v1.")
	}

	// Update file
	writeFile(t, path, updatedWorkflow)

	// Wait for reload
	deadline := time.Now().Add(3 * time.Second)
	for callCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}

	if callCount.Load() == 0 {
		t.Fatal("onChange callback was not called after file update")
	}

	cfg, prompt = cw.Current()
	if cfg.Polling.IntervalMs != 5000 {
		t.Errorf("reloaded interval = %d, want 5000", cfg.Polling.IntervalMs)
	}
	if prompt != "Prompt v2." {
		t.Errorf("reloaded prompt = %q, want %q", prompt, "Prompt v2.")
	}
}

func TestConfigWatcherInvalidReloadKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	writeFile(t, path, validWorkflow)

	var callCount atomic.Int32
	cw, err := NewConfigWatcher(path, func() {
		callCount.Add(1)
	})
	if err != nil {
		t.Fatalf("NewConfigWatcher: %v", err)
	}
	defer cw.Stop()

	// Write invalid content
	writeFile(t, path, invalidWorkflow)

	// Wait a bit for the watcher to process
	time.Sleep(500 * time.Millisecond)

	// onChange should NOT have been called for invalid reload
	if callCount.Load() != 0 {
		t.Errorf("onChange called %d times for invalid reload, want 0", callCount.Load())
	}

	// Previous config should be preserved
	cfg, prompt := cw.Current()
	if cfg.Tracker.Repo != "owner/repo" {
		t.Errorf("repo = %q, want %q", cfg.Tracker.Repo, "owner/repo")
	}
	if prompt != "Prompt v1." {
		t.Errorf("prompt = %q, want %q", prompt, "Prompt v1.")
	}
}

func TestConfigWatcherStop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	writeFile(t, path, validWorkflow)

	cw, err := NewConfigWatcher(path, nil)
	if err != nil {
		t.Fatalf("NewConfigWatcher: %v", err)
	}

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		cw.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestConfigWatcherConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	writeFile(t, path, validWorkflow)

	cw, err := NewConfigWatcher(path, nil)
	if err != nil {
		t.Fatalf("NewConfigWatcher: %v", err)
	}
	defer cw.Stop()

	// Concurrent reads should not race (run with -race)
	done := make(chan struct{})
	for range 10 {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					cw.Current()
				}
			}
		}()
	}

	// Trigger reload during concurrent reads
	writeFile(t, path, updatedWorkflow)
	time.Sleep(300 * time.Millisecond)
	close(done)
}
