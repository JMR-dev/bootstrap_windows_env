package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWingetInstallArgsUseExactNonInteractiveFlags(t *testing.T) {
	pkg := Package{Name: "Git", WingetID: "Git.Git", WingetSource: "winget"}
	want := []string{"install", "--id", "Git.Git", "--exact", "--source", "winget", "--accept-source-agreements", "--accept-package-agreements", "--disable-interactivity"}
	if got := WingetInstallArgs(pkg); !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestPreparePackageForInstallWritesVisualStudioConfigAndOverride(t *testing.T) {
	pkg := Package{
		Name:              "Visual Studio Community 2026",
		WingetID:          "Microsoft.VisualStudio.Community",
		WingetSource:      "winget",
		WingetOverride:    "--passive --config {config}",
		WingetConfigAsset: "assets/visual-studio-community.vsconfig",
	}
	prepared, err := PreparePackageForInstall(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prepared.WingetOverride, "{config}") {
		t.Fatalf("override still has placeholder: %q", prepared.WingetOverride)
	}
	args := WingetInstallArgs(prepared)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--override") || !strings.Contains(joined, "visual-studio-community.vsconfig") {
		t.Fatalf("winget args missing VS config override: %#v", args)
	}
}

func TestPreparePackageForInstallHandlesNoAssetAndMissingAsset(t *testing.T) {
	pkg := Package{Name: "Git", WingetID: "Git.Git", WingetSource: "winget"}
	prepared, err := PreparePackageForInstall(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prepared, pkg) {
		t.Fatalf("prepared = %#v, want unchanged package", prepared)
	}

	_, err = PreparePackageForInstall(Package{Name: "broken", WingetConfigAsset: "assets/missing.vsconfig"})
	if err == nil {
		t.Fatal("expected missing asset error")
	}
}

func TestWingetInstalledStateParsing(t *testing.T) {
	pkg := Package{WingetID: "Git.Git"}
	if !WingetOutputShowsInstalled(pkg, "Name Id Version\nGit Git.Git 2.0") {
		t.Fatal("expected installed output to be detected")
	}
	if WingetOutputShowsInstalled(pkg, "No installed package found matching input criteria.") {
		t.Fatal("expected missing package output to be false")
	}
	if WingetOutputShowsInstalled(pkg, "No package found matching input criteria.") {
		t.Fatal("expected missing package output to be false")
	}
}

func TestCheckPackagesKeepsStableOrderAndAggregatesErrors(t *testing.T) {
	a := Package{Name: "A", WingetID: "A.A", WingetSource: "winget"}
	b := Package{Name: "B", WingetID: "B.B", WingetSource: "winget"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("winget", WingetListArgs(a)...): {Stdout: "A A.A 1.0"},
		commandKey("winget", WingetListArgs(b)...): {Err: errors.New("boom"), Stderr: "source unavailable"},
	}}
	states := CheckPackages(context.Background(), runner, []Package{a, b}, 2)
	if len(states) != 2 || states[0].Package.Name != "A" || states[1].Package.Name != "B" {
		t.Fatalf("states not in input order: %#v", states)
	}
	if !states[0].Installed {
		t.Fatal("first package should be installed")
	}
	if states[1].CheckErr == nil {
		t.Fatal("second package should keep check error")
	}
}

func TestCheckPackagesUsesSingleWorkerMinimumAndMissingPackageIsPending(t *testing.T) {
	pkg := Package{Name: "Git", WingetID: "Git.Git", WingetSource: "winget"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("winget", WingetListArgs(pkg)...): {Err: errors.New("not found"), Stdout: "No installed package found matching input criteria."},
	}}
	states := CheckPackages(context.Background(), runner, []Package{pkg}, 0)
	if len(states) != 1 {
		t.Fatalf("states = %d, want 1", len(states))
	}
	if states[0].Installed || states[0].CheckErr != nil {
		t.Fatalf("state = %#v, want pending without check error", states[0])
	}
}

func TestInstallPackagesContinuesAfterFailure(t *testing.T) {
	a := Package{Name: "A", WingetID: "A.A", WingetSource: "winget"}
	b := Package{Name: "B", WingetID: "B.B", WingetSource: "winget"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("winget", WingetInstallArgs(a)...): {Err: errors.New("boom"), Stderr: "failed"},
		commandKey("winget", WingetInstallArgs(b)...): {},
	}, defaultErr: true}
	issues := InstallPackages(context.Background(), runner, []Package{a, b})
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	if !runner.called("winget", WingetInstallArgs(b)...) {
		t.Fatal("second package should still be attempted after first failure")
	}
}

func TestInstallPackagesReportsPrepareFailure(t *testing.T) {
	issues := InstallPackages(context.Background(), &fakeRunner{defaultErr: true}, []Package{{
		Name:              "broken",
		WingetConfigAsset: "assets/missing.vsconfig",
	}})
	if len(issues) != 1 || !strings.Contains(issues[0].Step, "prepare broken") {
		t.Fatalf("issues = %#v, want prepare failure", issues)
	}
}

func TestWriteInstallAssetReportsCacheDirectoryError(t *testing.T) {
	temp := t.TempDir()
	cacheFile := filepath.Join(temp, "cache-file")
	if err := os.WriteFile(cacheFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", cacheFile)
	t.Setenv("LocalAppData", cacheFile)
	if _, err := writeInstallAsset("assets/visual-studio-community.vsconfig"); err == nil {
		t.Fatal("expected cache directory error")
	}
}

func TestWriteInstallAssetFallsBackToTempDirWhenUserCacheIsUnavailable(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("LocalAppData", "")
	path, err := writeInstallAsset("assets/visual-studio-community.vsconfig")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "visual-studio-community.vsconfig" {
		t.Fatalf("path = %q", path)
	}
}

func TestWriteInstallAssetReportsWriteError(t *testing.T) {
	cache := t.TempDir()
	targetDir := filepath.Join(cache, "bootstrap_windows_env", "visual-studio-community.vsconfig")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("LocalAppData", cache)
	if _, err := writeInstallAsset("assets/visual-studio-community.vsconfig"); err == nil {
		t.Fatal("expected write error when target path is a directory")
	}
}
