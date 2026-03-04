package workspace

import (
	"context"
	"fmt"
	"time"
)

// HookConfig holds hook scripts and timeout.
type HookConfig struct {
	AfterCreate  string
	BeforeRun    string
	AfterRun     string
	BeforeRemove string
	TimeoutMs    int
}

// HookRunner executes lifecycle hook scripts.
type HookRunner struct {
	config   HookConfig
	executor Executor
}

// NewHookRunner creates a hook runner.
func NewHookRunner(config HookConfig, executor Executor) *HookRunner {
	return &HookRunner{config: config, executor: executor}
}

// RunHook executes a named hook in the given directory.
// Empty hook scripts are no-ops.
func (h *HookRunner) RunHook(ctx context.Context, name string, dir string) (string, error) {
	script := h.scriptFor(name)
	if script == "" {
		return "", nil
	}

	timeout := time.Duration(h.config.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := h.executor.RunCommand(ctx, dir, "bash", "-lc", script)
	if err != nil {
		return string(out), fmt.Errorf("hook %s: %w", name, err)
	}

	return string(out), nil
}

func (h *HookRunner) scriptFor(name string) string {
	switch name {
	case "after_create":
		return h.config.AfterCreate
	case "before_run":
		return h.config.BeforeRun
	case "after_run":
		return h.config.AfterRun
	case "before_remove":
		return h.config.BeforeRemove
	default:
		return ""
	}
}

// IsFatal returns true if failure of the named hook should be fatal.
func IsFatal(hookName string) bool {
	switch hookName {
	case "after_create", "before_run":
		return true
	default:
		return false
	}
}
