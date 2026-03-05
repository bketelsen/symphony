package main

import (
	"os"
	"testing"
)

func TestRunVersion(t *testing.T) {
	t.Parallel()

	err := run([]string{"--version"}, os.Stdout, os.Stderr)
	if err != nil {
		t.Errorf("run --version returned error: %v", err)
	}
}

func TestRunMissingWorkflow(t *testing.T) {
	t.Parallel()

	err := run([]string{"/nonexistent/WORKFLOW.md"}, os.Stdout, os.Stderr)
	if err == nil {
		t.Error("expected error for missing workflow file")
	}
}

func TestRunInvalidFlags(t *testing.T) {
	t.Parallel()

	err := run([]string{"--invalid-flag"}, os.Stdout, os.Stderr)
	if err == nil {
		t.Error("expected error for invalid flag")
	}
}
