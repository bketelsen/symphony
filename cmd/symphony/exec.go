package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/bjk/symphony/internal/agent"
)

// execCommandRunner implements tracker.CommandRunner using os/exec.
type execCommandRunner struct{}

func (e *execCommandRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("exec gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// shellExecutor implements workspace.Executor using os/exec.
type shellExecutor struct{}

func (e *shellExecutor) RunCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("exec %s %s: %w: %s", name, strings.Join(args, " "), err, out)
	}
	return out, nil
}

// execProcessRunner implements agent.ProcessRunner using os/exec.
type execProcessRunner struct{}

func (e *execProcessRunner) Start(ctx context.Context, cmd string, args []string, dir string) (agent.Process, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir

	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cmd, err)
	}

	return &execProcess{cmd: c, stdout: stdout, stderr: stderr}, nil
}

type execProcess struct {
	cmd    *exec.Cmd
	stdout io.Reader
	stderr io.Reader
}

func (p *execProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *execProcess) Stdout() io.Reader {
	return p.stdout
}

func (p *execProcess) Stderr() io.Reader {
	return p.stderr
}

// Ensure interfaces are satisfied at compile time.
var (
	_ = (*execCommandRunner)(nil)
	_ = (*shellExecutor)(nil)
	_ = (*execProcessRunner)(nil)
)

