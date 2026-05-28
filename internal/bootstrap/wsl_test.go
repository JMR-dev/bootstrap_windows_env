package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSelectOfficialFedoraChoosesCurrentSuccessor(t *testing.T) {
	output := `The following is a list of valid distributions:
FedoraLinux-42
FedoraLinux-43
Ubuntu`
	if got := SelectOfficialFedora(output); got != "FedoraLinux-43" {
		t.Fatalf("got %q, want FedoraLinux-43", got)
	}
}

func TestWSLStateTransitions(t *testing.T) {
	cases := []struct {
		name  string
		state WSLState
		want  WSLTransition
	}{
		{"absent", WSLState{}, WSLInstallPrerequisites},
		{"restart", WSLState{Installed: true, RestartRequired: true}, WSLWaitForRestart},
		{"set v2", WSLState{Installed: true}, WSLSetDefaultVersion},
		{"fedora missing", WSLState{Installed: true, Version2Default: true, FedoraAvailable: "FedoraLinux-42"}, WSLInstallFedora},
		{"first launch", WSLState{Installed: true, Version2Default: true, FedoraInstalled: "FedoraLinux-42"}, WSLWaitForFedoraUser},
		{"run guest", WSLState{Installed: true, Version2Default: true, FedoraInstalled: "FedoraLinux-42", FedoraReady: true}, WSLRunGuestBootstrap},
		{"complete", WSLState{Installed: true, Version2Default: true, FedoraInstalled: "FedoraLinux-42", FedoraReady: true, GuestConfigured: true}, WSLComplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextWSLTransition(tc.state); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGuestBootstrapInvocationPassesWSLAndNoAI(t *testing.T) {
	invocation := GuestBootstrapInvocation(Options{NoAI: true, LinuxReleaseRepo: "JMR-dev/bootstrap_dev_env"}, "FedoraLinux-42")
	joined := strings.Join(invocation.Args, " ")
	for _, want := range []string{"-d FedoraLinux-42", "JMR-dev/bootstrap_dev_env", "--wsl", "--headless", "--yes", "--no-ai"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("invocation missing %q: %s", want, joined)
		}
	}
	if !strings.Contains(joined, "sha256") {
		t.Fatal("guest script should verify checksum when a checksum asset is published")
	}
}

func TestRunWSLPhaseStopsForFedoraFirstLaunch(t *testing.T) {
	onlineArgs := []string{"--list", "--online"}
	installedArgs := []string{"--list", "--quiet"}
	probeArgs := []string{"-d", "FedoraLinux-42", "--exec", "sh", "-lc", "id -u >/dev/null"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"): {Stdout: "Default Version: 2"},
		commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { 'true' } else { 'false' }`): {Stdout: "false"},
		commandKey("wsl.exe", onlineArgs...):    {Stdout: "FedoraLinux-42"},
		commandKey("wsl.exe", installedArgs...): {Stdout: "FedoraLinux-42\n"},
		commandKey("wsl.exe", probeArgs...):     {Err: errors.New("first launch required")},
	}, defaultErr: true}
	result := RunWSLPhase(context.Background(), runner, Options{})
	if result.Issue != nil {
		t.Fatal(result.Issue)
	}
	if !strings.Contains(result.Notice, "first-launch") {
		t.Fatalf("notice = %q, want first-launch guidance", result.Notice)
	}
}

func TestRunWSLPhaseSetsDefaultVersion2(t *testing.T) {
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"): {Stdout: "Default Version: 1"},
		commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { 'true' } else { 'false' }`): {Stdout: "false"},
		commandKey("wsl.exe", "--list", "--online"): {Stdout: "FedoraLinux-42"},
		commandKey("wsl.exe", "--list", "--quiet"):  {Stdout: ""},
	}}
	result := RunWSLPhase(context.Background(), runner, Options{})
	if result.Issue != nil {
		t.Fatal(result.Issue)
	}
	if !runner.called("wsl.exe", "--set-default-version", "2") {
		t.Fatal("expected WSL default version to be set to 2")
	}
}

func TestRunWSLPhaseReportsGuestInvocationFailure(t *testing.T) {
	onlineArgs := []string{"--list", "--online"}
	installedArgs := []string{"--list", "--quiet"}
	probeArgs := []string{"-d", "FedoraLinux-42", "--exec", "sh", "-lc", "id -u >/dev/null"}
	markerArgs := []string{"-d", "FedoraLinux-42", "--exec", "sh", "-lc", "test -f ~/.local/state/bootstrap_dev_env/wsl-complete"}
	invocation := GuestBootstrapInvocation(Options{LinuxReleaseRepo: "JMR-dev/bootstrap_dev_env"}, "FedoraLinux-42")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"): {Stdout: "Default Version: 2"},
		commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { 'true' } else { 'false' }`): {Stdout: "false"},
		commandKey("wsl.exe", onlineArgs...):            {Stdout: "FedoraLinux-42"},
		commandKey("wsl.exe", installedArgs...):         {Stdout: "FedoraLinux-42\n"},
		commandKey("wsl.exe", probeArgs...):             {},
		commandKey("wsl.exe", markerArgs...):            {Err: errors.New("missing marker")},
		commandKey(invocation.Name, invocation.Args...): {Err: errors.New("download failed")},
	}}
	result := RunWSLPhase(context.Background(), runner, Options{LinuxReleaseRepo: "JMR-dev/bootstrap_dev_env"})
	if result.Issue == nil || !strings.Contains(result.Issue.Error(), "download failed") {
		t.Fatalf("issue = %v, want guest invocation failure", result.Issue)
	}
}

func TestDetectWSLCoversAbsentAndListFailures(t *testing.T) {
	state, err := DetectWSL(context.Background(), &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"): {Err: errors.New("not installed")},
	}, defaultErr: true})
	if err != nil {
		t.Fatal(err)
	}
	if state.Installed {
		t.Fatalf("state = %#v, want WSL absent", state)
	}

	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"):                     {Stdout: "Default Version: 2"},
		commandKey("powershell.exe", wslRebootProbeArgs()...): {Stdout: "false"},
		commandKey("wsl.exe", "--list", "--online"):           {Err: errors.New("offline"), Stderr: "network"},
	}, defaultErr: true}
	if _, err := DetectWSL(context.Background(), runner); err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("error = %v, want online list failure", err)
	}

	runner = &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"):                     {Stdout: "Default Version: 2"},
		commandKey("powershell.exe", wslRebootProbeArgs()...): {Stdout: "false"},
		commandKey("wsl.exe", "--list", "--online"):           {Stdout: "Ubuntu"},
	}, defaultErr: true}
	if _, err := DetectWSL(context.Background(), runner); err == nil || !strings.Contains(err.Error(), "no official FedoraLinux") {
		t.Fatalf("error = %v, want missing Fedora error", err)
	}

	runner = &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"):                     {Stdout: "Default Version: 2"},
		commandKey("powershell.exe", wslRebootProbeArgs()...): {Stdout: "false"},
		commandKey("wsl.exe", "--list", "--online"):           {Stdout: "FedoraLinux-42"},
		commandKey("wsl.exe", "--list", "--quiet"):            {Err: errors.New("list failed"), Stderr: "bad list"},
	}, defaultErr: true}
	if _, err := DetectWSL(context.Background(), runner); err == nil || !strings.Contains(err.Error(), "bad list") {
		t.Fatalf("error = %v, want installed list failure", err)
	}
}

func TestSelectOfficialFedoraReturnsEmptyWhenMissing(t *testing.T) {
	if got := SelectOfficialFedora("Ubuntu\nDebian"); got != "" {
		t.Fatalf("got %q, want empty result", got)
	}
}

func TestRunWSLPhaseInstallsPrerequisitesAndReportsFailures(t *testing.T) {
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"): {Err: errors.New("not installed")},
	}}
	result := RunWSLPhase(context.Background(), runner, Options{})
	if result.Issue != nil {
		t.Fatal(result.Issue)
	}
	if !runner.called("wsl.exe", "--install", "--no-distribution") || !strings.Contains(result.Notice, "Restart Windows") {
		t.Fatalf("result = %#v, want prerequisite install notice", result)
	}

	failRunner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("wsl.exe", "--status"):                       {Err: errors.New("not installed")},
		commandKey("wsl.exe", "--install", "--no-distribution"): {Err: errors.New("install failed"), Stderr: "denied"},
	}}
	result = RunWSLPhase(context.Background(), failRunner, Options{})
	if result.Issue == nil || !strings.Contains(result.Issue.Error(), "denied") {
		t.Fatalf("result = %#v, want prerequisite install failure", result)
	}
}

func TestRunWSLPhaseCoversRestartFedoraInstallGuestSuccessAndComplete(t *testing.T) {
	restartRunner := &fakeRunner{responses: wslDetectResponses("Default Version: 2", "true", "FedoraLinux-42", "", nil, nil)}
	result := RunWSLPhase(context.Background(), restartRunner, Options{})
	if result.Issue != nil || !strings.Contains(result.Notice, "pending restart") {
		t.Fatalf("result = %#v, want restart notice", result)
	}

	installRunner := &fakeRunner{responses: wslDetectResponses("Default Version: 2", "false", "FedoraLinux-42", "", nil, nil)}
	result = RunWSLPhase(context.Background(), installRunner, Options{})
	if result.Issue != nil {
		t.Fatal(result.Issue)
	}
	if !installRunner.called("wsl.exe", "--install", "--distribution", "FedoraLinux-42", "--no-launch") {
		t.Fatal("expected Fedora install command")
	}

	failInstall := wslDetectResponses("Default Version: 2", "false", "FedoraLinux-42", "", nil, nil)
	failInstall[commandKey("wsl.exe", "--install", "--distribution", "FedoraLinux-42", "--no-launch")] = CommandResult{Err: errors.New("install failed"), Stderr: "bad distro"}
	result = RunWSLPhase(context.Background(), &fakeRunner{responses: failInstall}, Options{})
	if result.Issue == nil || !strings.Contains(result.Issue.Error(), "bad distro") {
		t.Fatalf("result = %#v, want Fedora install failure", result)
	}

	opts := Options{LinuxReleaseRepo: "JMR-dev/bootstrap_dev_env"}
	invocation := GuestBootstrapInvocation(opts, "FedoraLinux-42")
	guestResponses := wslDetectResponses("Default Version: 2", "false", "FedoraLinux-42", "FedoraLinux-42\n", nil, errors.New("missing marker"))
	guestResponses[commandKey(invocation.Name, invocation.Args...)] = CommandResult{}
	result = RunWSLPhase(context.Background(), &fakeRunner{responses: guestResponses}, opts)
	if result.Issue != nil || !result.Done || !strings.Contains(result.Notice, "completed") {
		t.Fatalf("result = %#v, want guest success", result)
	}

	completeRunner := &fakeRunner{responses: wslDetectResponses("Default Version: 2", "false", "FedoraLinux-42", "FedoraLinux-42\n", nil, nil)}
	result = RunWSLPhase(context.Background(), completeRunner, Options{})
	if result.Issue != nil || !result.Done || !strings.Contains(result.Notice, "already provisioned") {
		t.Fatalf("result = %#v, want already complete", result)
	}
}

func TestRunWSLPhaseReportsSetDefaultFailure(t *testing.T) {
	responses := wslDetectResponses("Default Version: 1", "false", "FedoraLinux-42", "", nil, nil)
	responses[commandKey("wsl.exe", "--set-default-version", "2")] = CommandResult{Err: errors.New("set failed"), Stderr: "blocked"}
	result := RunWSLPhase(context.Background(), &fakeRunner{responses: responses}, Options{})
	if result.Issue == nil || !strings.Contains(result.Issue.Error(), "blocked") {
		t.Fatalf("result = %#v, want set default failure", result)
	}
}

func TestRunWSLPhaseReportsDetectionFailure(t *testing.T) {
	responses := wslDetectResponses("Default Version: 2", "false", "Ubuntu", "", nil, nil)
	result := RunWSLPhase(context.Background(), &fakeRunner{responses: responses}, Options{})
	if result.Issue == nil || !strings.Contains(result.Issue.Error(), "no official FedoraLinux") {
		t.Fatalf("result = %#v, want detection failure", result)
	}
}

func wslRebootProbeArgs() []string {
	return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { 'true' } else { 'false' }`}
}

func wslDetectResponses(statusOut, rebootOut, onlineOut, installedOut string, readyErr, markerErr error) map[string]CommandResult {
	responses := map[string]CommandResult{
		commandKey("wsl.exe", "--status"):                     {Stdout: statusOut},
		commandKey("powershell.exe", wslRebootProbeArgs()...): {Stdout: rebootOut},
		commandKey("wsl.exe", "--list", "--online"):           {Stdout: onlineOut},
		commandKey("wsl.exe", "--list", "--quiet"):            {Stdout: installedOut},
	}
	if strings.Contains(installedOut, "FedoraLinux-42") {
		responses[commandKey("wsl.exe", "-d", "FedoraLinux-42", "--exec", "sh", "-lc", "id -u >/dev/null")] = CommandResult{Err: readyErr}
		if readyErr == nil {
			responses[commandKey("wsl.exe", "-d", "FedoraLinux-42", "--exec", "sh", "-lc", "test -f ~/.local/state/bootstrap_dev_env/wsl-complete")] = CommandResult{Err: markerErr}
		}
	}
	return responses
}
