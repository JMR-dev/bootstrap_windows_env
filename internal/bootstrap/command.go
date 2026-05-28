package bootstrap

import (
	"bytes"
	"context"
	"fmt"
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
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("%s timed out after %s", name, timeout)
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
