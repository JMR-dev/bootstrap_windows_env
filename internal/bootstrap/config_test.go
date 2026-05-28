package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployManagedFileBacksUpExistingUnmanagedFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "profile.ps1")
	if err := os.WriteFile(target, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	deployment, err := DeployManagedFile("profile", target, []byte("managed"))
	if err != nil {
		t.Fatal(err)
	}
	if !deployment.Changed || deployment.Backup == "" {
		t.Fatalf("deployment = %#v, want changed with backup", deployment)
	}
	if got, _ := os.ReadFile(target); string(got) != "managed" {
		t.Fatalf("target = %q", got)
	}
	if got, _ := os.ReadFile(deployment.Backup); string(got) != "custom" {
		t.Fatalf("backup = %q", got)
	}

	deployment, err = DeployManagedFile("profile", target, []byte("managed"))
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Changed {
		t.Fatal("identical managed file should be left untouched")
	}
}

func TestDeployConfigAssetsWritesExpectedTargets(t *testing.T) {
	root := t.TempDir()
	paths := UserPaths{
		Home:         filepath.Join(root, "home"),
		LocalAppData: filepath.Join(root, "local"),
		Documents:    filepath.Join(root, "docs"),
	}
	deployments, err := DeployConfigAssets(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 3 {
		t.Fatalf("deployments = %d, want 3", len(deployments))
	}
	profile := filepath.Join(paths.Documents, "PowerShell", "Microsoft.PowerShell_profile.ps1")
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "oh-my-posh") || !strings.Contains(string(data), "fnm env") {
		t.Fatalf("profile missing expected initialization: %s", data)
	}
}

func TestDefaultUserPathsUsesHomeAndDocuments(t *testing.T) {
	paths, err := DefaultUserPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Home == "" || paths.LocalAppData == "" || paths.Documents == "" {
		t.Fatalf("paths should be populated: %#v", paths)
	}
	if !strings.HasSuffix(paths.Documents, "Documents") {
		t.Fatalf("documents path = %q, want Documents suffix", paths.Documents)
	}
}

func TestDefaultUserPathsFallsBackWhenLocalAppDataMissing(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	paths, err := DefaultUserPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(paths.Home, "AppData", "Local")
	if paths.LocalAppData != want {
		t.Fatalf("local app data = %q, want %q", paths.LocalAppData, want)
	}
}

func TestDeployConfigAssetsReportsDeploymentError(t *testing.T) {
	root := t.TempDir()
	homeFile := filepath.Join(root, "home")
	if err := os.WriteFile(homeFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := DeployConfigAssets(UserPaths{
		Home:         homeFile,
		LocalAppData: filepath.Join(root, "local"),
		Documents:    filepath.Join(root, "docs"),
	})
	if err == nil {
		t.Fatal("expected deployment error when home path is a file")
	}
}

func TestDeployManagedFileReportsReadErrorForDirectoryTarget(t *testing.T) {
	target := t.TempDir()
	if _, err := DeployManagedFile("directory", target, []byte("managed")); err == nil {
		t.Fatal("expected read error for directory target")
	}
}

func TestDeployManagedFileReportsDirectoryCreationError(t *testing.T) {
	root := t.TempDir()
	parentFile := filepath.Join(root, "parent")
	if err := os.WriteFile(parentFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parentFile, "profile.ps1")
	if _, err := DeployManagedFile("profile", target, []byte("managed")); err == nil {
		t.Fatal("expected directory creation error")
	}
}

func TestNextBackupPathUsesNumberedSuffix(t *testing.T) {
	target := filepath.Join(t.TempDir(), "profile.ps1")
	if err := os.WriteFile(target+".bak", []byte("backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".bak.1", []byte("backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := NextBackupPath(target), target+".bak.2"; got != want {
		t.Fatalf("backup path = %q, want %q", got, want)
	}
}

func TestConfigureNeovimLeavesManagedCheckoutAlone(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "local", "nvim")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("git", "-C", target, "config", "--get", "remote.origin.url"): {Stdout: "https://github.com/JMR-dev/nvim-config.git\n"},
	}, defaultErr: true}
	issue := ConfigureNeovim(context.Background(), runner, UserPaths{LocalAppData: filepath.Join(root, "local")})
	if !IsEmptyIssue(issue) {
		t.Fatalf("issue = %#v", issue)
	}
	if runner.called("git", "clone", "https://github.com/JMR-dev/nvim-config.git", target) {
		t.Fatal("managed checkout should not be cloned over")
	}
}

func TestConfigureNeovimClonesMissingConfig(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "local", "nvim")
	runner := &fakeRunner{}
	issue := ConfigureNeovim(context.Background(), runner, UserPaths{LocalAppData: filepath.Join(root, "local")})
	if !IsEmptyIssue(issue) {
		t.Fatalf("issue = %#v", issue)
	}
	if !runner.called("git", "clone", "https://github.com/JMR-dev/nvim-config.git", target) {
		t.Fatal("expected missing Neovim config to be cloned")
	}
}

func TestConfigureNeovimBacksUpUnmanagedConfig(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	target := filepath.Join(local, "nvim")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("git", "-C", target, "config", "--get", "remote.origin.url"): {Err: os.ErrNotExist},
	}}
	issue := ConfigureNeovim(context.Background(), runner, UserPaths{LocalAppData: local})
	if !IsEmptyIssue(issue) {
		t.Fatalf("issue = %#v", issue)
	}
	if _, err := os.Stat(target + ".bak"); err != nil {
		t.Fatalf("expected unmanaged config backup: %v", err)
	}
	if !runner.called("git", "clone", "https://github.com/JMR-dev/nvim-config.git", target) {
		t.Fatal("expected clone after backing up unmanaged config")
	}
}

func TestConfigureNeovimReportsCloneFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "local", "nvim")
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("git", "clone", "https://github.com/JMR-dev/nvim-config.git", target): {Err: os.ErrPermission, Stderr: "denied"},
	}}
	issue := ConfigureNeovim(context.Background(), runner, UserPaths{LocalAppData: filepath.Join(root, "local")})
	if IsEmptyIssue(issue) || !strings.Contains(issue.Err.Error(), "denied") {
		t.Fatalf("issue = %#v, want clone failure", issue)
	}
}

func TestFileSummaryCoversAllDeploymentStatuses(t *testing.T) {
	summary := FileSummary([]AssetDeployment{
		{Target: "same"},
		{Target: "new", Changed: true},
		{Target: "backup", Changed: true, Backup: "backup.bak"},
	})
	joined := strings.Join(summary, "\n")
	for _, want := range []string{"already managed", "written", "backed up"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("summary missing %q: %s", want, joined)
		}
	}
}

func TestPathContainsPathEntryUsesCleanCaseInsensitiveComparison(t *testing.T) {
	entry := filepath.Join("C:\\", "Tools", "bin")
	value := strings.Join([]string{filepath.Join("C:\\", "Other"), strings.ToLower(entry) + string(os.PathSeparator)}, string(os.PathListSeparator))
	if !pathContainsPathEntry(value, entry) {
		t.Fatalf("%q should contain %q", value, entry)
	}
	if pathContainsPathEntry(value, filepath.Join("C:\\", "Missing")) {
		t.Fatal("unexpected missing path match")
	}
}
