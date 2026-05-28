package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func WingetInstallArgs(pkg Package) []string {
	args := []string{
		"install", "--id", pkg.WingetID, "--exact", "--source", pkg.WingetSource,
		"--accept-source-agreements", "--accept-package-agreements", "--disable-interactivity",
	}
	if pkg.WingetOverride != "" {
		args = append(args, "--override", pkg.WingetOverride)
	}
	return args
}

func WingetListArgs(pkg Package) []string {
	return []string{
		"list", "--id", pkg.WingetID, "--exact", "--source", pkg.WingetSource,
		"--accept-source-agreements", "--disable-interactivity",
	}
}

func WingetOutputShowsInstalled(pkg Package, output string) bool {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "no installed package found") || strings.Contains(lower, "no package found") {
		return false
	}
	return strings.Contains(lower, strings.ToLower(pkg.WingetID))
}

type PackageState struct {
	Package   Package
	Installed bool
	CheckErr  error
}

func CheckPackages(ctx context.Context, runner Runner, packages []Package, workers int) []PackageState {
	if workers < 1 {
		workers = 1
	}
	states := make([]PackageState, len(packages))
	type job struct {
		index int
		pkg   Package
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobs {
				result := runner.Run(ctx, "winget", WingetListArgs(current.pkg)...)
				state := PackageState{Package: current.pkg}
				if result.Err != nil && !strings.Contains(strings.ToLower(result.CombinedOutput()), "no installed package found") {
					state.CheckErr = fmt.Errorf("%s: %w", result.CombinedOutput(), result.Err)
				} else {
					state.Installed = WingetOutputShowsInstalled(current.pkg, result.CombinedOutput())
				}
				states[current.index] = state
			}
		}()
	}
	for i, pkg := range packages {
		jobs <- job{index: i, pkg: pkg}
	}
	close(jobs)
	wg.Wait()
	return states
}

func PendingPackages(states []PackageState) []Package {
	var pending []Package
	for _, state := range states {
		if !state.Installed {
			pending = append(pending, state.Package)
		}
	}
	return pending
}

func UnknownPackageStates(packages []Package) []PackageState {
	states := make([]PackageState, 0, len(packages))
	for _, pkg := range packages {
		states = append(states, PackageState{Package: pkg})
	}
	return states
}

func InstallPackages(ctx context.Context, runner Runner, packages []Package) Issues {
	var issues Issues
	for _, pkg := range packages {
		prepared, err := PreparePackageForInstall(pkg)
		if err != nil {
			issues = append(issues, Issue{Step: "prepare " + pkg.Name, Err: err})
			continue
		}
		result := runner.Run(ctx, "winget", WingetInstallArgs(prepared)...)
		if result.Err != nil {
			issues = append(issues, Issue{
				Step: "install " + pkg.Name,
				Err:  fmt.Errorf("%w: %s", result.Err, result.CombinedOutput()),
			})
		}
	}
	return issues
}

func PreparePackageForInstall(pkg Package) (Package, error) {
	if pkg.WingetConfigAsset == "" {
		return pkg, nil
	}
	path, err := writeInstallAsset(pkg.WingetConfigAsset)
	if err != nil {
		return pkg, err
	}
	pkg.WingetOverride = strings.ReplaceAll(pkg.WingetOverride, "{config}", `"`+path+`"`)
	return pkg, nil
}

func writeInstallAsset(asset string) (string, error) {
	data, err := managedAssets.ReadFile(asset)
	if err != nil {
		return "", err
	}
	root, err := os.UserCacheDir()
	if err != nil || root == "" {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "bootstrap_windows_env")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, filepath.Base(asset))
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return target, nil
}
