package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/JMR-dev/bootstrap_windows_env/internal/bootstrap"
)

type fakeBootstrapApp struct {
	err  error
	opts bootstrap.Options
}

func (a *fakeBootstrapApp) Run(ctx context.Context, opts bootstrap.Options) error {
	a.opts = opts
	return a.err
}

func TestRunReturnsParseFactoryAndExecutionFailures(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"--only", "bad"}, &stderr, nil); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "invalid --only") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	if code := run([]string{"--dry-run"}, &stderr, func() (bootstrapApp, error) {
		return nil, errors.New("factory failed")
	}); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "factory failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	if code := run([]string{"--dry-run"}, &stderr, func() (bootstrapApp, error) {
		return &fakeBootstrapApp{err: errors.New("run failed")}, nil
	}); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "run failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunSuccessPassesParsedOptions(t *testing.T) {
	var stderr bytes.Buffer
	app := &fakeBootstrapApp{}
	code := run([]string{"--dry-run", "--no-wsl"}, &stderr, func() (bootstrapApp, error) {
		return app, nil
	})
	if code != 0 {
		t.Fatalf("code = %d stderr = %q, want success", code, stderr.String())
	}
	if !app.opts.DryRun || !app.opts.NoWSL {
		t.Fatalf("opts = %#v", app.opts)
	}
}

func TestMainDelegatesToRunAndExits(t *testing.T) {
	oldArgs := os.Args
	oldStderr := stderr
	oldExit := exitProcess
	oldNew := newBootstrapper
	defer func() {
		os.Args = oldArgs
		stderr = oldStderr
		exitProcess = oldExit
		newBootstrapper = oldNew
	}()

	var stderrBuffer bytes.Buffer
	os.Args = []string{"bootstrap_windows_env", "--only", "bad"}
	stderr = &stderrBuffer
	newBootstrapper = func() (bootstrapApp, error) {
		t.Fatal("newBootstrapper should not be called after parse failure")
		return nil, nil
	}
	exitProcess = func(code int) {
		panic(code)
	}

	defer func() {
		recovered := recover()
		if recovered != 2 {
			t.Fatalf("exit code = %#v, want 2", recovered)
		}
		if !strings.Contains(stderrBuffer.String(), "invalid --only") {
			t.Fatalf("stderr = %q", stderrBuffer.String())
		}
	}()
	main()
}
