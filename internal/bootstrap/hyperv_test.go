package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type staticRunner struct {
	result CommandResult
}

func (r staticRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	return r.result
}

func TestDetectHyperVParsesStatusAndReportsErrors(t *testing.T) {
	status, err := DetectHyperV(context.Background(), staticRunner{result: CommandResult{
		Stdout: "Edition=Windows 11 Pro\r\nSupported=True\r\nEnabled=True\r\nRestartRequired=True\r\nignored\r\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if status.Edition != "Windows 11 Pro" || !status.Supported || !status.Enabled || !status.RestartRequired {
		t.Fatalf("status = %#v", status)
	}

	_, err = DetectHyperV(context.Background(), staticRunner{result: CommandResult{Err: errors.New("blocked"), Stderr: "denied"}})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %v, want detect failure with output", err)
	}
}

func TestEnableHyperVCoversAllStates(t *testing.T) {
	cases := []struct {
		name   string
		status HyperVStatus
		want   string
	}{
		{"unsupported", HyperVStatus{Edition: "Windows 11 Home"}, "unavailable"},
		{"restart", HyperVStatus{Supported: true, RestartRequired: true}, "pending Windows restart"},
		{"enabled", HyperVStatus{Supported: true, Enabled: true}, "enabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			notice, err := EnableHyperV(context.Background(), &fakeRunner{defaultErr: true}, tc.status)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(notice, tc.want) {
				t.Fatalf("notice = %q, want %q", notice, tc.want)
			}
		})
	}

	runner := &fakeRunner{}
	notice, err := EnableHyperV(context.Background(), runner, HyperVStatus{Supported: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notice, "features were enabled") {
		t.Fatalf("notice = %q", notice)
	}
	if !runner.called("dism.exe", "/Online", "/Enable-Feature", "/FeatureName:Microsoft-Hyper-V-All", "/All", "/NoRestart") {
		t.Fatal("expected DISM enable command")
	}

	failRunner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("dism.exe", "/Online", "/Enable-Feature", "/FeatureName:Microsoft-Hyper-V-All", "/All", "/NoRestart"): {Err: errors.New("dism failed"), Stderr: "feature missing"},
	}}
	if _, err := EnableHyperV(context.Background(), failRunner, HyperVStatus{Supported: true}); err == nil || !strings.Contains(err.Error(), "feature missing") {
		t.Fatalf("error = %v, want DISM failure", err)
	}
}
