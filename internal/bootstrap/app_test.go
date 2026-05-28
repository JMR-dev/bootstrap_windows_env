package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithOSDoesNotProbePackagesBeforeWindowsUpdate(t *testing.T) {
	temp := t.TempDir()
	runner := &fakeRunner{}
	app := &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader(""),
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths: UserPaths{
			Home:         temp,
			LocalAppData: filepath.Join(temp, "AppData", "Local"),
			Documents:    filepath.Join(temp, "Documents"),
		},
	}

	if err := app.Run(context.Background(), Options{Yes: true, NoWSL: true, Headless: true}); err != nil {
		t.Fatal(err)
	}

	update := WindowsUpdateCommand()
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) == 0 {
		t.Fatal("expected runner calls")
	}
	if got, want := runner.calls[0], commandKey(update.Name, update.Args...); got != want {
		t.Fatalf("first command = %q, want Windows Update command %q", got, want)
	}
}

func TestRunWithDefaultsCanDryRunOnlyOS(t *testing.T) {
	temp := t.TempDir()
	app := &Bootstrapper{Paths: testUserPaths(temp)}
	if err := app.Run(context.Background(), Options{Only: PhaseOS, DryRun: true}); err != nil {
		t.Fatal(err)
	}
}

func TestRunStopsAfterOSRebootNotice(t *testing.T) {
	temp := t.TempDir()
	update := WindowsUpdateCommand()
	reboot := windowsRebootProbeCommand()
	restore := RestorePointCommand("bootstrap_windows_env: before OS configuration")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(update.Name, update.Args...): {},
		commandKey(reboot.Name, reboot.Args...): {Stdout: "true\n"},
	}, defaultErr: true}
	var out bytes.Buffer
	app := &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    &bytes.Buffer{},
		Paths:  testUserPaths(temp),
	}
	if err := app.Run(context.Background(), Options{Yes: true, NoWSL: true, Headless: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "require a reboot") {
		t.Fatalf("output missing reboot notice:\n%s", out.String())
	}
	if runner.called(restore.Name, restore.Args...) {
		t.Fatal("restore point should not run after reboot stop")
	}
}

func TestNewBootstrapperInitializesDefaults(t *testing.T) {
	app, err := NewBootstrapper()
	if err != nil {
		t.Fatal(err)
	}
	if app.Runner == nil || app.In == nil || app.Out == nil || app.Err == nil {
		t.Fatalf("bootstrapper defaults not populated: %#v", app)
	}
	if app.Paths.Home == "" || app.Paths.Documents == "" {
		t.Fatalf("paths not populated: %#v", app.Paths)
	}
}

func TestBuildPlanWrapperAndPrintPlanShowStatuses(t *testing.T) {
	pkg := Package{Name: "Git", WingetID: "Git.Git", WingetSource: "winget", Phase: PhaseHost}
	action := Action{Name: "done action"}
	plan := BuildPlan(context.Background(), &fakeRunner{}, Options{Only: PhaseOS})
	if len(plan.Phases) != 1 || plan.Phases[0] != PhaseOS {
		t.Fatalf("plan phases = %#v, want only OS", plan.Phases)
	}

	var out bytes.Buffer
	PrintPlan(&out, Plan{
		Phases: []Phase{PhaseHost, PhaseConfig, PhaseWSL},
		PackageStates: map[Phase][]PackageState{
			PhaseHost: {
				{Package: pkg, Installed: true},
				{Package: Package{Name: "Broken", WingetID: "Broken.ID"}, CheckErr: errors.New("check failed")},
			},
		},
		ActionStates: map[Phase][]ActionState{
			PhaseHost: {{Action: action, Installed: true}},
		},
		DeferredNativeWork: []string{"Docker Desktop"},
	}, Options{LinuxReleaseRepo: "JMR-dev/bootstrap_dev_env"})
	text := out.String()
	for _, want := range []string{"installed", "check failed; will retry install", "done action - done", "config: deploy managed", "official Fedora WSL", "Deferred native Windows workloads"} {
		if !strings.Contains(text, want) {
			t.Fatalf("plan output missing %q:\n%s", want, text)
		}
	}
}

func TestRunAbortWritesLogWithoutExecuting(t *testing.T) {
	temp := t.TempDir()
	runner := &fakeRunner{defaultErr: true}
	var out, errOut bytes.Buffer
	app := &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader("no\n"),
		Out:    &out,
		Err:    &errOut,
		Paths:  testUserPaths(temp),
	}
	if err := app.Run(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want none after abort", runner.calls)
	}
	if !strings.Contains(out.String(), "Aborted.") || !strings.Contains(out.String(), "Run log:") {
		t.Fatalf("output missing abort/log notice:\n%s", out.String())
	}
}

func TestRunReturnsLogWriteFailure(t *testing.T) {
	temp := t.TempDir()
	docFile := filepath.Join(temp, "Documents")
	if err := os.WriteFile(docFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &Bootstrapper{
		Runner: &fakeRunner{},
		In:     strings.NewReader(""),
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths: UserPaths{
			Home:         temp,
			LocalAppData: filepath.Join(temp, "local"),
			Documents:    docFile,
		},
	}
	if err := app.Run(context.Background(), Options{DryRun: true}); err == nil || !strings.Contains(err.Error(), "write run log") {
		t.Fatalf("error = %v, want log write failure", err)
	}
}

func TestRunOnlyHostSkipsOSAndExecutesHostPlan(t *testing.T) {
	temp := t.TempDir()
	runner := &fakeRunner{}
	var out bytes.Buffer
	app := &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    &bytes.Buffer{},
		Paths:  testUserPaths(temp),
	}
	if err := app.Run(context.Background(), Options{Only: PhaseHost, Yes: true, Headless: true}); err != nil {
		t.Fatal(err)
	}
	update := WindowsUpdateCommand()
	if runner.called(update.Name, update.Args...) {
		t.Fatal("host-only run should not execute OS phase")
	}
	if !strings.Contains(out.String(), "Run log:") {
		t.Fatalf("missing run log output:\n%s", out.String())
	}
}

func TestExecuteCheckedPlanRecordsWSLNoticeAndIssue(t *testing.T) {
	temp := t.TempDir()
	var out, errOut bytes.Buffer
	app := &Bootstrapper{
		Runner: &fakeRunner{responses: map[string]CommandResult{
			commandKey("wsl.exe", "--status"): {Err: errors.New("not installed")},
		}},
		Out:   &out,
		Err:   &errOut,
		Paths: testUserPaths(temp),
	}
	report := NewRunReport(Options{}, []Phase{PhaseWSL})
	issues := app.executeCheckedPlan(context.Background(), Plan{Phases: []Phase{PhaseWSL}}, Options{}, report)
	if err := issues.Err(); err != nil {
		t.Fatal(err)
	}
	if len(report.Notices) != 1 || !strings.Contains(out.String(), "WSL prerequisites") {
		t.Fatalf("notice/report mismatch: %#v\n%s", report.Notices, out.String())
	}

	app.Runner = &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"):                       {Err: errors.New("not installed")},
		commandKey("wsl.exe", "--install", "--no-distribution"): {Err: errors.New("blocked"), Stderr: "no admin"},
	}}
	report = NewRunReport(Options{}, []Phase{PhaseWSL})
	issues = app.executeCheckedPlan(context.Background(), Plan{Phases: []Phase{PhaseWSL}}, Options{}, report)
	if issues.Err() == nil || len(report.Issues) != 1 || !strings.Contains(errOut.String(), "ERROR: wsl") {
		t.Fatalf("issues = %#v report = %#v stderr = %s", issues, report.Issues, errOut.String())
	}
}

func TestExecuteCheckedPlanCoversOSStopAndHostConfigureIssue(t *testing.T) {
	temp := t.TempDir()
	update := WindowsUpdateCommand()
	reboot := windowsRebootProbeCommand()
	app := &Bootstrapper{
		Runner: &fakeRunner{responses: map[string]CommandResult{
			commandKey(update.Name, update.Args...): {},
			commandKey(reboot.Name, reboot.Args...): {Stdout: "true\n"},
		}, defaultErr: true},
		Out:   &bytes.Buffer{},
		Err:   &bytes.Buffer{},
		Paths: testUserPaths(temp),
	}
	report := NewRunReport(Options{}, []Phase{PhaseOS})
	issues := app.executeCheckedPlan(context.Background(), Plan{Phases: []Phase{PhaseOS}}, Options{}, report)
	if issues.Err() != nil {
		t.Fatal(issues.Err())
	}
	if len(report.Notices) != 1 || !strings.Contains(report.Events[len(report.Events)-1], "notice:") {
		t.Fatalf("report = %#v, want reboot notice and early return", report)
	}

	app.Runner = staticRunner{result: CommandResult{Err: errors.New("detect failed"), Stderr: "blocked"}}
	report = NewRunReport(Options{}, []Phase{PhaseHost})
	issues = app.executeCheckedPlan(context.Background(), Plan{Phases: []Phase{PhaseHost}}, Options{}, report)
	if len(issues) != 1 || issues[0].Step != "detect Hyper-V" {
		t.Fatalf("issues = %#v, want Hyper-V detect issue", issues)
	}
}

func TestRunConfigPhaseRecordsDeploymentIssue(t *testing.T) {
	temp := t.TempDir()
	homeFile := filepath.Join(temp, "home")
	if err := os.WriteFile(homeFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &Bootstrapper{
		Runner: &fakeRunner{},
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths: UserPaths{
			Home:         homeFile,
			LocalAppData: filepath.Join(temp, "local"),
			Documents:    filepath.Join(temp, "docs"),
		},
	}
	issues := app.runConfigPhase(context.Background(), Options{Headless: true})
	if len(issues) == 0 || !strings.Contains(issues[0].Step, "deploy managed config") {
		t.Fatalf("issues = %#v, want deployment issue", issues)
	}
}

func TestRunConfigPhaseRecordsNeovimIssue(t *testing.T) {
	temp := t.TempDir()
	target := filepath.Join(temp, "local", "nvim")
	app := &Bootstrapper{
		Runner: &fakeRunner{responses: map[string]CommandResult{
			commandKey("git", "clone", "https://github.com/JMR-dev/nvim-config.git", target): {Err: errors.New("clone failed"), Stderr: "offline"},
		}},
		Out:   &bytes.Buffer{},
		Err:   &bytes.Buffer{},
		Paths: testUserPaths(temp),
	}
	issues := app.runConfigPhase(context.Background(), Options{Headless: true})
	if len(issues) != 1 || issues[0].Step != "clone Neovim config" {
		t.Fatalf("issues = %#v, want Neovim clone issue", issues)
	}
}

func TestRunConfigPhaseOffersAuthWhenInteractive(t *testing.T) {
	temp := t.TempDir()
	pub := filepath.Join(temp, "home", ".ssh", "id_ed25519.pub")
	if err := os.MkdirAll(filepath.Dir(pub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pub, []byte("pub"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &Bootstrapper{
		Runner: &fakeRunner{},
		In:     strings.NewReader(""),
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths:  testUserPaths(temp),
	}
	if issues := app.runConfigPhase(context.Background(), Options{}); issues.Err() != nil {
		t.Fatalf("issues = %#v", issues)
	}
	if !app.Runner.(*fakeRunner).called("gh", "auth", "status") {
		t.Fatal("expected interactive auth status check")
	}
}

func TestConfigureHyperVRecordsDetectAndEnableFailures(t *testing.T) {
	app := &Bootstrapper{Runner: staticRunner{result: CommandResult{Err: errors.New("probe failed"), Stderr: "blocked"}}}
	if notice, issue := app.configureHyperV(context.Background()); notice != "" || issue == nil || issue.Step != "detect Hyper-V" {
		t.Fatalf("notice = %q issue = %#v, want detect issue", notice, issue)
	}

	app.Runner = &queueRunner{results: []CommandResult{
		{Stdout: "Edition=Windows 11 Pro\nSupported=True\nEnabled=False\nRestartRequired=False\n"},
		{Err: errors.New("dism failed"), Stderr: "feature unavailable"},
	}}
	if notice, issue := app.configureHyperV(context.Background()); notice != "" || issue == nil || issue.Step != "configure Hyper-V" {
		t.Fatalf("notice = %q issue = %#v, want configure issue", notice, issue)
	}

	app.Runner = &queueRunner{results: []CommandResult{
		{Stdout: "Edition=Windows 11 Pro\nSupported=True\nEnabled=False\nRestartRequired=False\n"},
		{},
	}}
	if notice, issue := app.configureHyperV(context.Background()); issue != nil || !strings.Contains(notice, "features were enabled") {
		t.Fatalf("notice = %q issue = %#v, want enable notice", notice, issue)
	}
}

func TestOfferInteractiveAuthCoversLoginFailure(t *testing.T) {
	temp := t.TempDir()
	host, _ := os.Hostname()
	email := os.Getenv("USERNAME") + "@" + host
	key := filepath.Join(temp, ".ssh", "id_ed25519")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("gh", "auth", "status"):                                                       {Err: errors.New("not logged in")},
		commandKey("gh", "auth", "login"):                                                        {Err: errors.New("login failed"), Stderr: "bad browser"},
		commandKey("ssh-keygen", "-t", "ed25519", "-C", email, "-f", key, "-N", ""):              {},
		commandKey("gh", "ssh-key", "add", key+".pub", "--title", "bootstrap_windows_env "+host): {Err: errors.New("upload failed"), Stderr: "denied"},
	}, defaultErr: true}
	app := &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader("y\n"),
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths:  UserPaths{Home: temp},
	}
	issues := app.offerInteractiveAuth(context.Background())
	if len(issues) != 1 || issues[0].Step != "gh auth login" {
		t.Fatalf("issues = %#v, want login failure", issues)
	}
}

func TestOfferInteractiveAuthCoversSSHUploadFailure(t *testing.T) {
	temp := t.TempDir()
	host, _ := os.Hostname()
	email := os.Getenv("USERNAME") + "@" + host
	key := filepath.Join(temp, ".ssh", "id_ed25519")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("gh", "auth", "status"):                                                       {},
		commandKey("ssh-keygen", "-t", "ed25519", "-C", email, "-f", key, "-N", ""):              {},
		commandKey("gh", "ssh-key", "add", key+".pub", "--title", "bootstrap_windows_env "+host): {Err: errors.New("upload failed"), Stderr: "denied"},
	}, defaultErr: true}
	app := &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader("y\n"),
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths:  UserPaths{Home: temp},
	}
	issues := app.offerInteractiveAuth(context.Background())
	if len(issues) != 1 || issues[0].Step != "gh ssh-key add" {
		t.Fatalf("issues = %#v, want upload failure", issues)
	}
	if !runner.called("ssh-keygen", "-t", "ed25519", "-C", email, "-f", key, "-N", "") {
		t.Fatal("expected ssh-keygen command")
	}
}

func TestOfferInteractiveAuthCoversSSHDirectoryAndKeygenFailures(t *testing.T) {
	temp := t.TempDir()
	homeFile := filepath.Join(temp, "home")
	if err := os.WriteFile(homeFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &Bootstrapper{
		Runner: &fakeRunner{responses: map[string]CommandResult{
			commandKey("gh", "auth", "status"): {},
		}},
		In:    strings.NewReader("y\n"),
		Out:   &bytes.Buffer{},
		Err:   &bytes.Buffer{},
		Paths: UserPaths{Home: homeFile},
	}
	issues := app.offerInteractiveAuth(context.Background())
	if len(issues) != 1 || issues[0].Step != "create SSH directory" {
		t.Fatalf("issues = %#v, want SSH directory failure", issues)
	}

	host, _ := os.Hostname()
	email := os.Getenv("USERNAME") + "@" + host
	key := filepath.Join(temp, "home2", ".ssh", "id_ed25519")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("gh", "auth", "status"):                                          {},
		commandKey("ssh-keygen", "-t", "ed25519", "-C", email, "-f", key, "-N", ""): {Err: errors.New("keygen failed"), Stderr: "bad key"},
	}, defaultErr: true}
	app = &Bootstrapper{
		Runner: runner,
		In:     strings.NewReader("y\n"),
		Out:    &bytes.Buffer{},
		Err:    &bytes.Buffer{},
		Paths:  UserPaths{Home: filepath.Join(temp, "home2")},
	}
	issues = app.offerInteractiveAuth(context.Background())
	if len(issues) != 1 || issues[0].Step != "ssh-keygen" {
		t.Fatalf("issues = %#v, want ssh-keygen failure", issues)
	}
	if runner.called("gh", "ssh-key", "add", key+".pub", "--title", "bootstrap_windows_env "+host) {
		t.Fatal("ssh key should not be uploaded after key generation failure")
	}
}

func TestConfirmHandlesYesNoAndMissingInput(t *testing.T) {
	if !confirm(strings.NewReader("YES\n"), &bytes.Buffer{}, "prompt") {
		t.Fatal("YES should confirm")
	}
	if confirm(strings.NewReader("n\n"), &bytes.Buffer{}, "prompt") {
		t.Fatal("n should not confirm")
	}
	if confirm(strings.NewReader(""), &bytes.Buffer{}, "prompt") {
		t.Fatal("missing input should not confirm")
	}
}

func TestRecordIssuesEmptyAndHasPhaseFalse(t *testing.T) {
	app := &Bootstrapper{Err: &bytes.Buffer{}}
	report := NewRunReport(Options{}, nil)
	var issues Issues
	app.recordIssues(report, &issues, nil)
	if len(issues) != 0 || len(report.Issues) != 0 {
		t.Fatalf("issues = %#v report = %#v, want no changes", issues, report.Issues)
	}
	if hasPhase([]Phase{PhaseHost}, PhaseOS) {
		t.Fatal("hasPhase should be false for missing phase")
	}
}

func testUserPaths(root string) UserPaths {
	return UserPaths{
		Home:         filepath.Join(root, "home"),
		LocalAppData: filepath.Join(root, "local"),
		Documents:    filepath.Join(root, "Documents"),
	}
}

type queueRunner struct {
	results []CommandResult
}

func (r *queueRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	if len(r.results) == 0 {
		return CommandResult{}
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result
}
