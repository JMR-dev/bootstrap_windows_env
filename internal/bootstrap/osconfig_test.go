package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWinUtilCommandUsesManagedConfigAndRunFlag(t *testing.T) {
	cmd := WinUtilCommand()
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"christitus.com/win", "-Config", "winutil-sane-default.json", "-Run"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("WinUtil command missing %q: %s", want, joined)
		}
	}
}

func TestPowerPolicyCommandContainsDesktopAndLaptopSettings(t *testing.T) {
	cmd := PowerPolicyCommand()
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"Invoke-Native 'powercfg.exe' @('/hibernate', 'on')",
		"@('/setacvalueindex', $scheme, 'SUB_VIDEO', 'VIDEOIDLE', '900')",
		"@('/setdcvalueindex', $scheme, 'SUB_VIDEO', 'VIDEOIDLE', '300')",
		"@('/setacvalueindex', $scheme, 'SUB_SLEEP', 'STANDBYIDLE', '0')",
		"@('/setdcvalueindex', $scheme, 'SUB_SLEEP', 'STANDBYIDLE', '900')",
		"'LIDACTION', '0'",
		"'LIDACTION', '1'",
		"'PBUTTONACTION', '3'",
		"'SBUTTONACTION', '1'",
		"InactivityTimeoutSecs",
		"SCHEME_MIN",
		"SCHEME_MAX",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("power policy command missing %q", want)
		}
	}
	if strings.Contains(joined, "SCHEME_CURRENT") {
		t.Fatal("power policy should configure explicit schemes, not SCHEME_CURRENT")
	}
}

func TestRunOSPhaseStopsBeforeRestorePointWhenRebootIsRequired(t *testing.T) {
	update := WindowsUpdateCommand()
	reboot := windowsRebootProbeCommand()
	restore := RestorePointCommand("bootstrap_windows_env: before OS configuration")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(update.Name, update.Args...): {},
		commandKey(reboot.Name, reboot.Args...): {Stdout: "true\n"},
	}, defaultErr: true}
	result := RunOSPhase(context.Background(), runner)
	if !result.Stop {
		t.Fatal("expected OS phase to stop for pending reboot")
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", result.Issues)
	}
	if runner.called(restore.Name, restore.Args...) {
		t.Fatal("restore point should not be attempted before required reboot")
	}
}

func TestRunOSPhaseRunsSecondRestorePointAfterConfiguration(t *testing.T) {
	runner := &fakeRunner{}
	result := RunOSPhase(context.Background(), runner)
	if result.Stop {
		t.Fatalf("OS phase unexpectedly stopped: %#v", result)
	}
	if err := result.Issues.Err(); err != nil {
		t.Fatal(err)
	}
	first := RestorePointCommand("bootstrap_windows_env: before OS configuration")
	second := RestorePointCommand("bootstrap_windows_env: before package installation")
	if !runner.called(first.Name, first.Args...) || !runner.called(second.Name, second.Args...) {
		t.Fatal("expected both restore point commands to run")
	}
}

func TestRunOSPhaseContinuesAfterNonRestoreFailure(t *testing.T) {
	update := WindowsUpdateCommand()
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(update.Name, update.Args...): {Err: context.Canceled, Stderr: "update failed"},
	}}
	result := RunOSPhase(context.Background(), runner)
	if result.Stop {
		t.Fatalf("non-restore failure should be reported but should not stop the OS phase: %#v", result)
	}
	if result.Issues.Err() == nil {
		t.Fatal("expected Windows Update issue")
	}
	second := RestorePointCommand("bootstrap_windows_env: before package installation")
	if !runner.called(second.Name, second.Args...) {
		t.Fatal("expected final restore point after non-restore failure")
	}
}

func TestRunOSPhaseStopsWhenSecondRestorePointFails(t *testing.T) {
	second := RestorePointCommand("bootstrap_windows_env: before package installation")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(second.Name, second.Args...): {Err: context.Canceled, Stderr: "restore failed"},
	}}
	result := RunOSPhase(context.Background(), runner)
	if !result.Stop {
		t.Fatal("expected OS phase to stop when final restore point fails")
	}
	if result.Issues.Err() == nil {
		t.Fatal("expected restore point issue")
	}
}

func TestRunOSPhaseStopsWhenFirstRestorePointFails(t *testing.T) {
	first := RestorePointCommand("bootstrap_windows_env: before OS configuration")
	winutil := WinUtilCommand()
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(first.Name, first.Args...): {Err: errors.New("restore failed"), Stderr: "blocked"},
	}}
	result := RunOSPhase(context.Background(), runner)
	if !result.Stop {
		t.Fatal("expected first restore point failure to stop OS phase")
	}
	if result.Issues.Err() == nil {
		t.Fatal("expected restore point issue")
	}
	if runner.called(winutil.Name, winutil.Args...) {
		t.Fatal("WinUtil should not run after first restore point failure")
	}
}

func TestRunOSPhaseRecordsWinUtilFailureAndContinuesToFinalRestorePoint(t *testing.T) {
	winutil := WinUtilCommand()
	second := RestorePointCommand("bootstrap_windows_env: before package installation")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(winutil.Name, winutil.Args...): {Err: errors.New("winutil failed"), Stderr: "script failed"},
	}}
	result := RunOSPhase(context.Background(), runner)
	if result.Stop {
		t.Fatalf("WinUtil failure should not stop OS phase: %#v", result)
	}
	if result.Issues.Err() == nil {
		t.Fatal("expected WinUtil issue")
	}
	if !runner.called(second.Name, second.Args...) {
		t.Fatal("expected final restore point after WinUtil failure")
	}
}

func TestDetectWindowsRebootRequiredReportsProbeFailure(t *testing.T) {
	_, err := DetectWindowsRebootRequired(context.Background(), &fakeRunner{defaultErr: true})
	if err == nil || !strings.Contains(err.Error(), "unexpected command") {
		t.Fatalf("error = %v, want probe failure", err)
	}
}

func TestRunOSPhaseRecordsRebootProbeFailureAndContinues(t *testing.T) {
	reboot := windowsRebootProbeCommand()
	second := RestorePointCommand("bootstrap_windows_env: before package installation")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(reboot.Name, reboot.Args...): {Err: errors.New("probe failed"), Stderr: "bad registry"},
	}}
	result := RunOSPhase(context.Background(), runner)
	if result.Stop {
		t.Fatalf("reboot probe failure should be recorded but should not stop OS phase: %#v", result)
	}
	if result.Issues.Err() == nil {
		t.Fatal("expected reboot probe issue")
	}
	if !runner.called(second.Name, second.Args...) {
		t.Fatal("expected final restore point after reboot probe failure")
	}
}

func TestRestorePointCommandDisablesFrequencyThrottle(t *testing.T) {
	cmd := RestorePointCommand("bootstrap_windows_env: test")
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"SystemRestorePointCreationFrequency", "Checkpoint-Computer"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("restore point command missing %q", want)
		}
	}
}

func TestPolicyAndExplorerCommandsCoverRequestedRegistrySettings(t *testing.T) {
	policy := strings.Join(WindowsUpdatePolicyCommand().Args, " ")
	for _, want := range []string{"DeferFeatureUpdatesPeriodInDays", "90", "ActiveHoursStart", "ActiveHoursEnd", "AllowMUUpdateService"} {
		if !strings.Contains(policy, want) {
			t.Fatalf("policy command missing %q", want)
		}
	}
	explorer := strings.Join(ExplorerThemeCommand().Args, " ")
	for _, want := range []string{"86ca1aa0-34aa-4e8b-a509-50c905bae2a2", "AppsUseLightTheme", "SystemUsesLightTheme"} {
		if !strings.Contains(explorer, want) {
			t.Fatalf("explorer/theme command missing %q", want)
		}
	}
}

func TestDeveloperModeCommandEnablesAnywhereInstallAndWindowsSudo(t *testing.T) {
	cmd := DeveloperModeCommand()
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"AppModelUnlock",
		"AllowDevelopmentWithoutDevLicense",
		"AllowAllTrustedApps",
		"DeveloperSettings",
		"EnableSudo",
		"SudoMode",
		"sudo.exe config --enable normal",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("developer mode command missing %q", want)
		}
	}
	if !strings.Contains(joined, "$LASTEXITCODE") {
		t.Fatal("developer mode command should check sudo.exe exit code")
	}
}

func TestOSCommandsCheckNativeExitCodes(t *testing.T) {
	for name, cmd := range map[string]CommandSpec{
		"Windows Update": WindowsUpdateCommand(),
		"Vivaldi":        VivaldiDefaultBrowserCommand(),
		"Store":          StoreUpdateCommand(),
	} {
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, "$LASTEXITCODE") && !strings.Contains(joined, "ResultCode") {
			t.Fatalf("%s command should check native or COM result codes", name)
		}
	}
}

func TestMustInstallAssetPathFallsBackToInputWhenAssetIsMissing(t *testing.T) {
	if got := mustInstallAssetPath("assets/not-present.json"); got != "assets/not-present.json" {
		t.Fatalf("got %q, want original asset path", got)
	}
}

func windowsRebootProbeCommand() CommandSpec {
	return powerShell(`$paths = @(
  'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending',
  'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'
)
$pending = $false
foreach ($path in $paths) {
    if (Test-Path $path) { $pending = $true }
}
$sessionManager = Get-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager' -Name PendingFileRenameOperations -ErrorAction SilentlyContinue
if ($sessionManager.PendingFileRenameOperations) { $pending = $true }
if ($pending) { 'true' } else { 'false' }`)
}
