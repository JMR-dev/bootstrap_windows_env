package bootstrap

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var managedAssets embed.FS

type UserPaths struct {
	Home         string
	LocalAppData string
	Documents    string
}

func DefaultUserPaths() (UserPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return UserPaths{}, err
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(home, "AppData", "Local")
	}
	documents := filepath.Join(home, "Documents")
	return UserPaths{Home: home, LocalAppData: local, Documents: documents}, nil
}

type AssetDeployment struct {
	Name    string
	Target  string
	Backup  string
	Changed bool
}

func ConfigTargets(paths UserPaths) map[string]string {
	return map[string]string{
		"assets/.wezterm.lua":                     filepath.Join(paths.Home, ".wezterm.lua"),
		"assets/Microsoft.PowerShell_profile.ps1": filepath.Join(paths.Documents, "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		"assets/jmr.omp.json":                     filepath.Join(paths.Home, ".config", "oh-my-posh", "jmr.omp.json"),
	}
}

func DeployConfigAssets(paths UserPaths) ([]AssetDeployment, error) {
	targets := ConfigTargets(paths)
	order := []string{"assets/.wezterm.lua", "assets/Microsoft.PowerShell_profile.ps1", "assets/jmr.omp.json"}
	deployments := make([]AssetDeployment, 0, len(order))
	for _, name := range order {
		data, err := fs.ReadFile(managedAssets, name)
		if err != nil {
			return deployments, err
		}
		deployment, err := DeployManagedFile(name, targets[name], data)
		if err != nil {
			return deployments, err
		}
		deployments = append(deployments, deployment)
	}
	return deployments, nil
}

func DeployManagedFile(name, target string, data []byte) (AssetDeployment, error) {
	deployment := AssetDeployment{Name: name, Target: target}
	existing, err := os.ReadFile(target)
	if err == nil {
		if bytes.Equal(existing, data) {
			return deployment, nil
		}
		deployment.Backup = NextBackupPath(target)
		if err := os.Rename(target, deployment.Backup); err != nil {
			return deployment, fmt.Errorf("back up %s: %w", target, err)
		}
	} else if !os.IsNotExist(err) {
		return deployment, fmt.Errorf("read %s: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return deployment, err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return deployment, fmt.Errorf("write %s: %w", target, err)
	}
	deployment.Changed = true
	return deployment, nil
}

func NextBackupPath(target string) string {
	base := target + ".bak"
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s.%d", base, n)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func ConfigureNeovim(ctx context.Context, runner Runner, paths UserPaths) Issue {
	target := filepath.Join(paths.LocalAppData, "nvim")
	if _, err := os.Stat(target); err == nil {
		remote := runner.Run(ctx, "git", "-C", target, "config", "--get", "remote.origin.url")
		if remote.Err == nil && strings.Contains(strings.ToLower(remote.Stdout), "jmr-dev/nvim-config") {
			return Issue{}
		}
		backup := NextBackupPath(target)
		if err := os.Rename(target, backup); err != nil {
			return Issue{Step: "back up Neovim config", Err: err}
		}
	}
	result := runner.Run(ctx, "git", "clone", "https://github.com/JMR-dev/nvim-config.git", target)
	if result.Err != nil {
		return Issue{Step: "clone Neovim config", Err: fmt.Errorf("%w: %s", result.Err, result.CombinedOutput())}
	}
	return Issue{}
}

func IsEmptyIssue(issue Issue) bool {
	return issue.Step == "" && issue.Err == nil
}

func FileSummary(deployments []AssetDeployment) []string {
	var summary []string
	for _, deployment := range deployments {
		switch {
		case !deployment.Changed:
			summary = append(summary, deployment.Target+" (already managed)")
		case deployment.Backup != "":
			summary = append(summary, deployment.Target+" (backed up to "+deployment.Backup+")")
		default:
			summary = append(summary, deployment.Target+" (written)")
		}
	}
	return summary
}

func pathContainsPathEntry(value, entry string) bool {
	for _, part := range strings.Split(value, string(os.PathListSeparator)) {
		if strings.EqualFold(filepath.Clean(part), filepath.Clean(entry)) {
			return true
		}
	}
	return false
}
