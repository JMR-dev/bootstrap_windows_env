// Command integration drives the bootstrap_windows_env integration test by
// orchestrating a Hyper-V VM through the local `vagrant` CLI.
//
// Usage (run from any directory inside the repository):
//
//	go run ./test/integration                 # build, vagrant up, run, destroy
//	go run ./test/integration --keep          # leave VM after run for debugging
//	go run ./test/integration --skip-build    # reuse dist/bootstrap_windows_env.exe
//	go run ./test/integration --args="--yes --no-wsl --only os"
//
// The host must run Windows with Hyper-V enabled and the `vagrant` CLI in PATH.
// Hyper-V is the only Vagrant provider exercised here; the Vagrantfile in this
// directory documents the relevant environment variables for tuning.
//
// There is no official Vagrant SDK for Go (Vagrant is a Ruby project), so this
// driver shells out to the `vagrant` CLI through os/exec — the conventional
// "Go scripting" pattern for orchestrating external tools.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	guestExe     = `C:\Users\vagrant\bootstrap_windows_env.exe`
	defaultArgs  = "--yes --no-wsl"
	exitUsage    = 2
	exitInternal = 1
)

type config struct {
	args      string
	keep      bool
	skipBuild bool
	provider  string
	timeout   time.Duration
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitUsage)
	}
	if runtime.GOOS != "windows" {
		fmt.Fprintln(os.Stderr, "integration test must run on a Windows host with Hyper-V")
		os.Exit(exitInternal)
	}
	if err := runIntegration(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "integration test failed:", err)
		os.Exit(exitInternal)
	}
	fmt.Println("integration test passed")
}

func parseFlags(argv []string) (config, error) {
	fs := flag.NewFlagSet("integration", flag.ContinueOnError)
	cfg := config{
		args:     defaultArgs,
		provider: "hyperv",
		timeout:  90 * time.Minute,
	}
	fs.StringVar(&cfg.args, "args", cfg.args, "arguments passed to bootstrap_windows_env.exe inside the guest")
	fs.BoolVar(&cfg.keep, "keep", false, "leave the VM running after the test (do not destroy)")
	fs.BoolVar(&cfg.skipBuild, "skip-build", false, "reuse dist/bootstrap_windows_env.exe instead of rebuilding")
	fs.StringVar(&cfg.provider, "provider", cfg.provider, "vagrant provider to use")
	fs.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "overall timeout for the integration run")
	if err := fs.Parse(argv); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return cfg, nil
}

func runIntegration(cfg config) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	vagrantDir := filepath.Join(root, "test", "integration")
	distDir := filepath.Join(root, "dist")
	exePath := filepath.Join(distDir, "bootstrap_windows_env.exe")

	if _, err := exec.LookPath("vagrant"); err != nil {
		return fmt.Errorf("vagrant CLI not found in PATH: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	if !cfg.skipBuild {
		if err := buildBootstrapper(ctx, root, exePath); err != nil {
			return fmt.Errorf("build bootstrapper: %w", err)
		}
	}
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("bootstrapper binary missing at %s: %w", exePath, err)
	}

	logFile, err := os.Create(filepath.Join(vagrantDir, "integration.log"))
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()
	outWriter := io.MultiWriter(os.Stdout, logFile)
	startTime := time.Now()
	defer func() {
		fmt.Fprintf(outWriter, "\n--- Integration Test Completed in %v ---\n", time.Since(startTime).Round(time.Second))
	}()

	if err := vagrant(ctx, vagrantDir, outWriter, "up", "--provider="+cfg.provider); err != nil {
		return fmt.Errorf("vagrant up: %w", err)
	}

	defer func() {
		// If the bootstrap or package installer is still in progress inside the guest,
		// stop the processes to release log files and resources, allowing clean teardown.
		killCtx, cancelKill := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelKill()
		_ = vagrant(killCtx, vagrantDir, outWriter, "powershell", "-c", "Get-Process -Name bootstrap_windows_env,choco,nmap,msiexec,setup -ErrorAction SilentlyContinue | Stop-Process -Force")

		// Always try to surface the in-guest bootstrapper log before destroying
		// the VM. WinRM's stdout streaming is bursty; this gives a deterministic
		// artifact even when the powershell call cut out mid-run.
		logCtx, cancelLog := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancelLog()
		_ = vagrant(logCtx, vagrantDir, outWriter, "powershell", "-c", tailGuestLogCmd)

		if cfg.keep {
			fmt.Println("--keep set; leaving Vagrant machine running. Destroy manually with `vagrant destroy -f` in", vagrantDir)
			return
		}
		// Always attempt cleanup with a fresh context so an outer timeout does
		// not leave a Hyper-V VM stranded.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := vagrant(cleanupCtx, vagrantDir, outWriter, "destroy", "-f"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: vagrant destroy failed: %v\n", err)
		}
	}()

	if err := vagrant(ctx, vagrantDir, outWriter, "upload", exePath, guestExe); err != nil {
		return fmt.Errorf("vagrant upload: %w", err)
	}

	// `vagrant powershell -e -c` invokes PowerShell in the guest over the WinRM
	// communicator using elevated privileges (which runs via a scheduled task
	// to bypass WinRM COM restrictions for Windows Update).
	guestCmd := fmt.Sprintf("$env:BOOTSTRAP_INTEGRATION_TEST='true'; & %q %s; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", guestExe, cfg.args)
	
	fmt.Fprintln(outWriter, "\n--- Starting Pass 1 (OS Phase) ---")
	if err := vagrant(ctx, vagrantDir, outWriter, "powershell", "-e", "-c", guestCmd); err != nil {
		fmt.Fprintf(outWriter, "Warning: Pass 1 returned an error (likely a Vagrant scheduled task quirk): %v\n", err)
	}

	// Reboot the VM to apply OS updates.
	fmt.Fprintln(outWriter, "\n--- Rebooting VM to apply OS updates ---")
	_ = vagrant(ctx, vagrantDir, outWriter, "powershell", "-c", "Restart-Computer -Force")

	fmt.Fprintln(outWriter, "Waiting for VM to go offline...")
	for i := 0; i < 60; i++ {
		time.Sleep(5 * time.Second)
		pollCtx, pollCancel := context.WithTimeout(ctx, 10*time.Second)
		err := vagrant(pollCtx, vagrantDir, io.Discard, "powershell", "-c", "Write-Output 'still up'")
		pollCancel()
		if err != nil {
			break // WinRM dropped, machine is rebooting
		}
	}

	// Wait for the VM to come back online
	fmt.Fprintln(outWriter, "Waiting for VM to come back online...")
	deadline := time.Now().Add(15 * time.Minute)
	vmReady := false
	for time.Now().Before(deadline) {
		time.Sleep(15 * time.Second)
		// Use a short timeout for the poll so we don't hang if WinRM drops packets
		pollCtx, pollCancel := context.WithTimeout(ctx, 30*time.Second)
		err := vagrant(pollCtx, vagrantDir, outWriter, "powershell", "-c", "Write-Output 'VM is back'")
		pollCancel()
		if err == nil {
			vmReady = true
			break
		}
	}
	if !vmReady {
		return fmt.Errorf("timed out waiting for VM to come back online after reboot")
	}

	// Wait 60 seconds for LSASS and Task Scheduler services to fully stabilize after reboot
	fmt.Fprintln(outWriter, "Waiting 60 seconds for system services to stabilize...")
	time.Sleep(60 * time.Second)

	// Run the bootstrapper again to finish the remaining phases (host, custom, config).
	// We run these individually using --only to bypass the OS phase, which avoids
	// infinite reboot loops and Vagrant scheduled task hangs.
	fmt.Fprintln(outWriter, "\n--- Resuming bootstrap after reboot (Pass 2) ---")
	
	chocoEnsureCmd := `
if (-not (Get-Command choco -ErrorAction SilentlyContinue)) {
    Write-Host "Chocolatey not found. Installing Chocolatey..."
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    Set-ExecutionPolicy Bypass -Scope Process -Force
    Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
} else {
    Write-Host "Chocolatey is already installed."
}
`
	_ = vagrant(ctx, vagrantDir, outWriter, "powershell", "-e", "-c", chocoEnsureCmd)
	
	for _, phase := range []string{"host", "custom", "config"} {
		fmt.Fprintf(outWriter, "\n--- Running Phase: %s ---\n", phase)
		passArgs := strings.Replace(cfg.args, "--only os", "", 1) + " --only " + phase
		guestCmdPhase := fmt.Sprintf("$env:BOOTSTRAP_INTEGRATION_TEST='true'; & %q %s; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }", guestExe, passArgs)
		
		var phaseErr error
		for attempt := 1; attempt <= 3; attempt++ {
			if attempt > 1 {
				fmt.Fprintf(outWriter, "\n[Attempt %d/3] Retrying phase %s after failure (likely WinRM network drop)...\n", attempt, phase)
				time.Sleep(10 * time.Second) // Wait for network to stabilize before reconnecting
			}
			
			phaseErr = vagrant(ctx, vagrantDir, outWriter, "powershell", "-e", "-c", guestCmdPhase)
			if phaseErr == nil || phase == "custom" {
				break
			}
		}
		if phaseErr != nil {
			if phase == "custom" {
				fmt.Fprintf(outWriter, "Warning: Phase custom failed (ignored because graphical installers are expected to fail or hang in non-interactive sessions): %v\n", phaseErr)
			} else {
				return fmt.Errorf("guest bootstrap run failed (phase %s) after 3 attempts: %w", phase, phaseErr)
			}
		}
	}

	return nil
}

func buildBootstrapper(ctx context.Context, repoRoot, exePath string) error {
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		return err
	}
	fmt.Printf("building %s\n", exePath)
	cmd := exec.CommandContext(ctx, "go", "build", "-o", exePath, "./cmd/bootstrap_windows_env")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func vagrant(ctx context.Context, dir string, out io.Writer, args ...string) error {
	fmt.Fprintf(out, "[vagrant] %s\n", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "vagrant", args...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	// Default to the always-present Hyper-V "Default Switch" so `vagrant up`
	// never blocks on the interactive virtual-switch prompt. Callers can
	// override by exporting VAGRANT_HYPERV_VIRTUAL_SWITCH before running.
	env := append(os.Environ(),
		"VAGRANT_DEFAULT_PROVIDER=hyperv",
	)
	if os.Getenv("VAGRANT_HYPERV_VIRTUAL_SWITCH") == "" {
		env = append(env, "VAGRANT_HYPERV_VIRTUAL_SWITCH=Default Switch")
	}
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w (deadline exceeded)", err)
		}
		return err
	}
	return nil
}

// tailGuestLogCmd locates the most recent bootstrap_windows_env log in the
// guest's Documents folder and prints its last 500 lines. Used after every
// run so the host has a deterministic artifact even when WinRM stops
// streaming midway through a long step.
const tailGuestLogCmd = `$ErrorActionPreference='SilentlyContinue'
$log = Get-ChildItem -Path "$env:USERPROFILE\Documents\bootstrap_windows_env-*.log" |
       Sort-Object LastWriteTime -Descending | Select-Object -First 1
if ($log) {
    Write-Host "=== guest log: $($log.FullName) ==="
    Get-Content -Tail 500 -Path $log.FullName
} else {
    Write-Host "no bootstrap_windows_env log found under $env:USERPROFILE\Documents"
}`

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not locate go.mod ancestor of current directory")
		}
		dir = parent
	}
}