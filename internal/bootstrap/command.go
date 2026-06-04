package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type CommandResult struct {
	Command string
	Stdout  string
	Stderr  string
	Err     error
}

func (r CommandResult) CombinedOutput() string {
	return strings.TrimSpace(strings.TrimSpace(r.Stdout) + "\n" + strings.TrimSpace(r.Stderr))
}

type Runner interface {
	Run(ctx context.Context, name string, args ...string) CommandResult
}

type ExecRunner struct {
	Timeout time.Duration
	Stream  io.Writer
}

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
		return cmd.Process.Kill()
	}
	if r.Stream != nil {
		cmd.Stdout = io.MultiWriter(r.Stream, &stdout)
		cmd.Stderr = io.MultiWriter(r.Stream, &stderr)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("%s timed out", name)
	}
	return CommandResult{
		Command: name + " " + strings.Join(args, " "),
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Err:     err,
	}
}

type Issue struct {
	Step string
	Err  error
}

type Issues []Issue

func (issues Issues) Err() error {
	if len(issues) == 0 {
		return nil
	}
	var parts []string
	for _, issue := range issues {
		parts = append(parts, fmt.Sprintf("%s: %v", issue.Step, issue.Err))
	}
	return fmt.Errorf("%d step(s) failed: %s", len(issues), strings.Join(parts, "; "))
}
