package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type HyperVStatus struct {
	Edition         string
	Supported       bool
	Enabled         bool
	RestartRequired bool
}

func DetectHyperV(ctx context.Context, runner Runner) (HyperVStatus, error) {
	script := `$edition=(Get-ComputerInfo -Property WindowsProductName).WindowsProductName; ` +
		`$supported=($edition -match 'Pro|Enterprise|Education'); ` +
		`$enabled=$false; if ($supported) { $feature=Get-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All -ErrorAction SilentlyContinue; $enabled=($feature.State -eq 'Enabled') }; ` +
		`$reboot=(Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'); ` +
		`Write-Output ('Edition='+$edition); Write-Output ('Supported='+$supported); Write-Output ('Enabled='+$enabled); Write-Output ('RestartRequired='+$reboot)`
	result := runner.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	if result.Err != nil {
		return HyperVStatus{}, fmt.Errorf("detect Hyper-V: %w: %s", result.Err, result.CombinedOutput())
	}
	values := parseValues(result.Stdout)
	return HyperVStatus{
		Edition:         values["Edition"],
		Supported:       strings.EqualFold(values["Supported"], "True"),
		Enabled:         strings.EqualFold(values["Enabled"], "True"),
		RestartRequired: strings.EqualFold(values["RestartRequired"], "True"),
	}, nil
}

func EnableHyperV(ctx context.Context, runner Runner, status HyperVStatus) (string, error) {
	if !status.Supported {
		return "Vagrant installed, but Hyper-V provider setup is unavailable on Windows " + status.Edition + "; no alternate provider is substituted.", nil
	}
	if status.RestartRequired {
		return "A pending Windows restart must be completed before Hyper-V-backed Vagrant setup can continue.", nil
	}
	if status.Enabled {
		return "Hyper-V is enabled for the Vagrant provider.", nil
	}
	result := runner.Run(ctx, "dism.exe", "/Online", "/Enable-Feature", "/FeatureName:Microsoft-Hyper-V-All", "/All", "/NoRestart")
	if result.Err != nil {
		var exitErr *exec.ExitError
		if errors.As(result.Err, &exitErr) && exitErr.ExitCode() == 3010 {
			// 3010 is ERROR_SUCCESS_REBOOT_REQUIRED
		} else {
			return "", fmt.Errorf("enable Hyper-V: %w: %s", result.Err, result.CombinedOutput())
		}
	}
	return "Hyper-V features were enabled; restart Windows and rerun to complete Vagrant provider setup.", nil
}

func parseValues(output string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			values[key] = value
		}
	}
	return values
}
