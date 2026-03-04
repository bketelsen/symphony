package workspace

import (
	"context"
	"fmt"
	"testing"
)

func TestRunHookEmpty(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	hr := NewHookRunner(HookConfig{}, exec)

	out, err := hr.RunHook(context.Background(), "after_create", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("output = %q, want empty", out)
	}
	if len(exec.calls) != 0 {
		t.Errorf("got %d calls, want 0 for empty hook", len(exec.calls))
	}
}

func TestRunHookExecutes(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	hr := NewHookRunner(HookConfig{
		AfterCreate: "npm install",
		TimeoutMs:   5000,
	}, exec)

	_, err := hr.RunHook(context.Background(), "after_create", "/tmp/ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(exec.calls))
	}
	call := exec.calls[0]
	if call.Dir != "/tmp/ws" {
		t.Errorf("dir = %q, want /tmp/ws", call.Dir)
	}
	if call.Name != "bash" {
		t.Errorf("name = %q, want bash", call.Name)
	}
	if call.Args[0] != "-lc" || call.Args[1] != "npm install" {
		t.Errorf("args = %v, want [-lc, npm install]", call.Args)
	}
}

func TestRunHookError(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{err: fmt.Errorf("exit 1")}
	hr := NewHookRunner(HookConfig{
		BeforeRun: "git pull",
		TimeoutMs: 5000,
	}, exec)

	_, err := hr.RunHook(context.Background(), "before_run", "/tmp")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunHookUnknownName(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	hr := NewHookRunner(HookConfig{AfterCreate: "echo hi"}, exec)

	_, err := hr.RunHook(context.Background(), "nonexistent", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error for unknown hook: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("got %d calls, want 0 for unknown hook", len(exec.calls))
	}
}

func TestIsFatal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hook string
		want bool
	}{
		{"after_create is fatal", "after_create", true},
		{"before_run is fatal", "before_run", true},
		{"after_run is not fatal", "after_run", false},
		{"before_remove is not fatal", "before_remove", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsFatal(tt.hook); got != tt.want {
				t.Errorf("IsFatal(%q) = %v, want %v", tt.hook, got, tt.want)
			}
		})
	}
}
