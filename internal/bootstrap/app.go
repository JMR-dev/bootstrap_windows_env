package bootstrap

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Bootstrapper struct {
	Runner Runner
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	Paths  UserPaths
}

func NewBootstrapper() (*Bootstrapper, error) {
	paths, err := DefaultUserPaths()
	if err != nil {
		return nil, err
	}
	return &Bootstrapper{
		Runner: ExecRunner{Timeout: 45 * time.Minute},
		In:     os.Stdin,
		Out:    os.Stdout,
		Err:    os.Stderr,
		Paths:  paths,
	}, nil
}

type Plan struct {
	Phases             []Phase
	PackageStates      map[Phase][]PackageState
	ActionStates       map[Phase][]ActionState
	DeferredNativeWork []string
}

func BuildPlan(ctx context.Context, runner Runner, opts Options) Plan {
	return BuildPlanForPhases(ctx, runner, opts, SelectedPhases(opts), true)
}

func BuildPlanForPhases(ctx context.Context, runner Runner, opts Options, phases []Phase, checkState bool) Plan {
	plan := Plan{
		Phases:             append([]Phase(nil), phases...),
		PackageStates:      make(map[Phase][]PackageState),
		ActionStates:       make(map[Phase][]ActionState),
		DeferredNativeWork: DeferredWindowsTools,
	}
	for _, phase := range plan.Phases {
		packages := PackagesForPhase(opts, phase)
		if len(packages) > 0 {
			if checkState {
				plan.PackageStates[phase] = CheckPackages(ctx, runner, packages, 8)
			} else {
				plan.PackageStates[phase] = UnknownPackageStates(packages)
			}
		}
		switch phase {
		case PhaseHost:
			actions := HostActions()
			if checkState {
				plan.ActionStates[phase] = CheckActions(ctx, runner, actions)
			} else {
				plan.ActionStates[phase] = UnknownActionStates(actions)
			}
		case PhaseCustom:
			actions := CustomActions(opts)
			if checkState {
				plan.ActionStates[phase] = CheckActions(ctx, runner, actions)
			} else {
				plan.ActionStates[phase] = UnknownActionStates(actions)
			}
		}
	}
	return plan
}

func PrintPlan(out io.Writer, plan Plan, opts Options) {
	fmt.Fprintln(out, "Windows bootstrap plan")
	for _, phase := range plan.Phases {
		fmt.Fprintf(out, "\n[%s]\n", phase)
		states := plan.PackageStates[phase]
		if len(states) > 0 {
			for _, state := range states {
				status := "pending"
				if state.Installed {
					status = "installed"
				} else if state.CheckErr != nil {
					status = "check failed; will retry install"
				}
				fmt.Fprintf(out, "  winget: %s (%s) - %s\n", state.Package.Name, state.Package.WingetID, status)
			}
		}
		actions := plan.ActionStates[phase]
		for _, state := range actions {
			status := "pending"
			if state.Installed {
				status = "done"
			}
			fmt.Fprintf(out, "  action: %s - %s\n", state.Action.Name, status)
		}
		if phase == PhaseOS {
			fmt.Fprintln(out, "  os: install Windows/Microsoft updates and stop before packages if reboot is required")
			fmt.Fprintln(out, "  os: create restore point, run WinUtil config, apply developer/power/update/UI/store/taskbar defaults")
			fmt.Fprintln(out, "  os: create second restore point before host package installation")
		}
		if phase == PhaseConfig {
			fmt.Fprintln(out, "  config: deploy managed WezTerm, PowerShell, oh-my-posh, and Neovim config")
		}
		if phase == PhaseWSL {
			fmt.Fprintf(out, "  wsl: install official Fedora WSL and invoke %s release with --wsl\n", opts.LinuxReleaseRepo)
		}
	}
	if len(plan.DeferredNativeWork) > 0 && opts.Includes(PhaseHost) {
		fmt.Fprintf(out, "\nDeferred native Windows workloads: %s\n", strings.Join(plan.DeferredNativeWork, ", "))
	}
}

func (b *Bootstrapper) Run(ctx context.Context, opts Options) (runErr error) {
	if b.Runner == nil {
		b.Runner = ExecRunner{Timeout: 45 * time.Minute}
	}
	if b.In == nil {
		b.In = os.Stdin
	}
	if b.Out == nil {
		b.Out = os.Stdout
	}
	if b.Err == nil {
		b.Err = os.Stderr
	}

	phases := SelectedPhases(opts)
	report := NewRunReport(opts, phases)
	defer func() {
		report.Finish(runErr)
		logPath, err := WriteRunLog(b.Paths, report)
		if err != nil {
			fmt.Fprintf(b.Err, "ERROR: write run log: %v\n", err)
			if runErr == nil {
				runErr = fmt.Errorf("write run log: %w", err)
			}
			return
		}
		fmt.Fprintf(b.Out, "Run log: %s\n", logPath)
	}()

	checkState := opts.DryRun || !hasPhase(phases, PhaseOS)
	plan := BuildPlanForPhases(ctx, b.Runner, opts, phases, checkState)
	PrintPlan(b.Out, plan, opts)
	if opts.DryRun {
		return nil
	}
	if !opts.Yes {
		if !confirm(b.In, b.Out, "Proceed with this plan? [y/N] ") {
			fmt.Fprintln(b.Out, "Aborted.")
			report.Event("aborted by user")
			return nil
		}
	}

	var issues Issues
	if hasPhase(phases, PhaseOS) {
		result := b.runOSPhase(ctx, report)
		b.recordIssues(report, &issues, result.Issues)
		if result.Stop {
			report.Event("stopped after OS phase")
			return issues.Err()
		}

		remaining := phasesWithout(phases, PhaseOS)
		if len(remaining) > 0 {
			report.Event("checking post-OS package and action state")
			plan = BuildPlanForPhases(ctx, b.Runner, opts, remaining, true)
			PrintPlan(b.Out, plan, opts)
			issues = append(issues, b.executeCheckedPlan(ctx, plan, opts, report)...)
		}
		return issues.Err()
	}

	issues = append(issues, b.executeCheckedPlan(ctx, plan, opts, report)...)
	return issues.Err()
}

func (b *Bootstrapper) runOSPhase(ctx context.Context, report *RunReport) OSPhaseResult {
	report.Event("phase os started")
	result := RunOSPhase(ctx, b.Runner)
	for _, notice := range result.Notices {
		b.recordNotice(report, notice)
	}
	if !result.Stop {
		report.Event("phase os completed")
	}
	return result
}

func (b *Bootstrapper) executeCheckedPlan(ctx context.Context, plan Plan, opts Options, report *RunReport) Issues {
	var issues Issues
	for _, phase := range plan.Phases {
		report.Event("phase %s started", phase)
		switch phase {
		case PhaseOS:
			result := b.runOSPhase(ctx, report)
			b.recordIssues(report, &issues, result.Issues)
			if result.Stop {
				return issues
			}
		case PhaseHost:
			b.recordIssues(report, &issues, InstallPackages(ctx, b.Runner, PendingPackages(plan.PackageStates[phase])))
			if notice, issue := b.configureHyperV(ctx); issue != nil {
				b.recordIssues(report, &issues, Issues{*issue})
			} else if notice != "" {
				b.recordNotice(report, notice)
			}
			b.recordIssues(report, &issues, ExecuteActions(ctx, b.Runner, PendingActions(plan.ActionStates[phase])))
		case PhaseCustom:
			b.recordIssues(report, &issues, InstallPackages(ctx, b.Runner, PendingPackages(plan.PackageStates[phase])))
			b.recordIssues(report, &issues, ExecuteActions(ctx, b.Runner, PendingActions(plan.ActionStates[phase])))
		case PhaseConfig:
			b.recordIssues(report, &issues, b.runConfigPhase(ctx, opts))
		case PhaseWSL:
			result := RunWSLPhase(ctx, b.Runner, opts)
			if result.Issue != nil {
				b.recordIssues(report, &issues, Issues{{Step: "wsl", Err: result.Issue}})
			}
			if result.Notice != "" {
				b.recordNotice(report, result.Notice)
			}
		}
		report.Event("phase %s completed", phase)
	}
	return issues
}

func (b *Bootstrapper) recordNotice(report *RunReport, notice string) {
	fmt.Fprintln(b.Out, notice)
	report.Notice(notice)
}

func (b *Bootstrapper) recordIssues(report *RunReport, all *Issues, issues Issues) {
	if len(issues) == 0 {
		return
	}
	*all = append(*all, issues...)
	for _, issue := range issues {
		fmt.Fprintf(b.Err, "ERROR: %s: %v\n", issue.Step, issue.Err)
		report.Issue(issue)
	}
}

func hasPhase(phases []Phase, target Phase) bool {
	for _, phase := range phases {
		if phase == target {
			return true
		}
	}
	return false
}

func phasesWithout(phases []Phase, target Phase) []Phase {
	filtered := make([]Phase, 0, len(phases))
	for _, phase := range phases {
		if phase != target {
			filtered = append(filtered, phase)
		}
	}
	return filtered
}

func (b *Bootstrapper) configureHyperV(ctx context.Context) (string, *Issue) {
	status, err := DetectHyperV(ctx, b.Runner)
	if err != nil {
		issue := Issue{Step: "detect Hyper-V", Err: err}
		return "", &issue
	}
	notice, err := EnableHyperV(ctx, b.Runner, status)
	if err != nil {
		issue := Issue{Step: "configure Hyper-V", Err: err}
		return "", &issue
	}
	return notice, nil
}

func (b *Bootstrapper) runConfigPhase(ctx context.Context, opts Options) Issues {
	var issues Issues
	deployments, err := DeployConfigAssets(b.Paths)
	if err != nil {
		issues = append(issues, Issue{Step: "deploy managed config", Err: err})
	} else {
		for _, line := range FileSummary(deployments) {
			fmt.Fprintln(b.Out, line)
		}
	}
	issue := ConfigureNeovim(ctx, b.Runner, b.Paths)
	if !IsEmptyIssue(issue) {
		issues = append(issues, issue)
	}
	if !opts.Headless {
		issues = append(issues, b.offerInteractiveAuth(ctx)...)
	}
	return issues
}

func (b *Bootstrapper) offerInteractiveAuth(ctx context.Context) Issues {
	var issues Issues
	if result := b.Runner.Run(ctx, "gh", "auth", "status"); result.Err != nil {
		if confirm(b.In, b.Out, "Run `gh auth login` now? [y/N] ") {
			login := b.Runner.Run(ctx, "gh", "auth", "login")
			if login.Err != nil {
				issues = append(issues, Issue{Step: "gh auth login", Err: fmt.Errorf("%w: %s", login.Err, login.CombinedOutput())})
			}
		}
	}
	key := filepath.Join(b.Paths.Home, ".ssh", "id_ed25519")
	pub := key + ".pub"
	if _, err := os.Stat(pub); os.IsNotExist(err) {
		if confirm(b.In, b.Out, "Create and upload an ed25519 Git SSH key with gh? [y/N] ") {
			if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
				issues = append(issues, Issue{Step: "create SSH directory", Err: err})
				return issues
			}
			host, _ := os.Hostname()
			email := os.Getenv("USERNAME") + "@" + host
			gen := b.Runner.Run(ctx, "ssh-keygen", "-t", "ed25519", "-C", email, "-f", key, "-N", "")
			if gen.Err != nil {
				issues = append(issues, Issue{Step: "ssh-keygen", Err: fmt.Errorf("%w: %s", gen.Err, gen.CombinedOutput())})
				return issues
			}
			add := b.Runner.Run(ctx, "gh", "ssh-key", "add", pub, "--title", "bootstrap_windows_env "+host)
			if add.Err != nil {
				issues = append(issues, Issue{Step: "gh ssh-key add", Err: fmt.Errorf("%w: %s", add.Err, add.CombinedOutput())})
			}
		}
	}
	return issues
}

func confirm(in io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes"
}
