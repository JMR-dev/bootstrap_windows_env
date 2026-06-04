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

func init() {
	chocoCommand = "choco"
}

func TestChocoInstallArgsUseExactNonInteractiveFlags(t *testing.T) {
	pkg := Package{Name: "Git", ChocoID: "git"}
	want := []string{"install", "git", "-y", "--no-progress"}
	if got := ChocoInstallArgs(pkg); !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestPreparePackageForInstallWritesVisualStudioConfigAndOverride(t *testing.T) {
	pkg := Package{
		Name:             "Visual Studio Community 2026",
		ChocoID:          "visualstudio2022community",
		InstallerArgs:    "--passive --config {config}",
		ChocoConfigAsset: "assets/visual-studio-community.vsconfig",
	}
	prepared, err := PreparePackageForInstall(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prepared.InstallerArgs, "{config}") {
		t.Fatalf("override still has placeholder: %q", prepared.InstallerArgs)
	}
	args := ChocoInstallArgs(prepared)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--install-arguments") || !strings.Contains(joined, "visual-studio-community.vsconfig") {
		t.Fatalf("choco args missing VS config override: %#v", args)
	}
}

func TestPreparePackageForInstallHandlesNoAssetAndMissingAsset(t *testing.T) {
	pkg := Package{Name: "Git", ChocoID: "git"}
	prepared, err := PreparePackageForInstall(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prepared, pkg) {
		t.Fatalf("prepared = %#v, want unchanged package", prepared)
	}

	_, err = PreparePackageForInstall(Package{Name: "broken", ChocoConfigAsset: "assets/missing.vsconfig"})
	if err == nil {
		t.Fatal("expected missing asset error")
	}
}

func TestChocoInstalledStateParsing(t *testing.T) {
	pkg := Package{ChocoID: "git"}
	if !ChocoOutputShowsInstalled(pkg, "Chocolatey v1.2.1\ngit 2.40.1\n1 packages installed.") {
		t.Fatal("expected installed output to be detected")
	}
	if ChocoOutputShowsInstalled(pkg, "Chocolatey v1.2.1\n0 packages installed.") {
		t.Fatal("expected missing package output to be false")
	}
}

func TestCheckPackagesKeepsStableOrderAndAggregatesErrors(t *testing.T) {
	a := Package{Name: "A", ChocoID: "a"}
	b := Package{Name: "B", ChocoID: "b"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Test-Path C:\\ProgramData\\chocolatey\\lib\\a"): {Stdout: "True\n"},
		commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Test-Path C:\\ProgramData\\chocolatey\\lib\\b"): {Err: errors.New("boom"), Stderr: "source unavailable"},
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
	pkg := Package{Name: "Git", ChocoID: "git"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Test-Path C:\\ProgramData\\chocolatey\\lib\\git"): {Stdout: "False\n"},
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
	a := Package{Name: "A", ChocoID: "a"}
	b := Package{Name: "B", ChocoID: "b"}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey("choco", ChocoInstallArgs(a)...): {Err: errors.New("boom"), Stderr: "failed"},
		commandKey("choco", ChocoInstallArgs(b)...): {},
	}, defaultErr: true}
	issues := InstallPackages(context.Background(), runner, []Package{a, b})
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	if !runner.called("choco", ChocoInstallArgs(b)...) {
		t.Fatal("second package should still be attempted after first failure")
	}
}

func TestInstallPackagesReportsPrepareFailure(t *testing.T) {
	issues := InstallPackages(context.Background(), &fakeRunner{defaultErr: true}, []Package{{
		Name:             "broken",
		ChocoConfigAsset: "assets/missing.vsconfig",
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

func TestCheckPackagesNvidiaDriver(t *testing.T) {
	// Backup original functions and defer restore
	oldFetch := fetchNvidiaDriverURLFn
	oldGetInstalled := getInstalledNvidiaVersionFn
	defer func() {
		fetchNvidiaDriverURLFn = oldFetch
		getInstalledNvidiaVersionFn = oldGetInstalled
	}()

	// Mock fetchNvidiaDriverURLFn to return a version
	fetchNvidiaDriverURLFn = func(pfid string, driverType string) (string, string, error) {
		if pfid != "995" {
			t.Errorf("expected pfid 995, got %s", pfid)
		}
		if driverType != "Studio" {
			t.Errorf("expected driverType Studio, got %s", driverType)
		}
		return "https://uk.download.nvidia.com/Windows/610.47/610.47-desktop-win10-win11-64bit-international-nsd-dch-whql.exe", "610.47", nil
	}

	// 1. Test case: Installed version matches API version -> should report installed = true
	getInstalledNvidiaVersionFn = func(ctx context.Context, runner Runner) (string, error) {
		return "610.47", nil
	}

	pkg := Package{
		Name:          "Nvidia Driver",
		ChocoParams:   "RTX 4090",
		InstallerArgs: "Studio",
	}

	runner := &fakeRunner{}
	states := CheckPackages(context.Background(), runner, []Package{pkg}, 1)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if !states[0].Installed {
		t.Error("expected Nvidia Driver to be reported as installed")
	}
	if states[0].CheckErr != nil {
		t.Errorf("expected no check error, got %v", states[0].CheckErr)
	}

	// 2. Test case: Installed version does not match API version -> should report installed = false
	getInstalledNvidiaVersionFn = func(ctx context.Context, runner Runner) (string, error) {
		return "596.36", nil
	}

	states = CheckPackages(context.Background(), runner, []Package{pkg}, 1)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if states[0].Installed {
		t.Error("expected Nvidia Driver to be reported as not installed (pending)")
	}
	if states[0].CheckErr != nil {
		t.Errorf("expected no check error, got %v", states[0].CheckErr)
	}
}

func TestInstallPackagesNvidiaDriver(t *testing.T) {
	// Backup original functions and defer restore
	oldFetch := fetchNvidiaDriverURLFn
	defer func() {
		fetchNvidiaDriverURLFn = oldFetch
	}()

	fetchNvidiaDriverURLFn = func(pfid string, driverType string) (string, string, error) {
		return "https://mock.nvidia.download/driver.exe", "610.47", nil
	}

	pkg := Package{
		Name:          "Nvidia Driver",
		ChocoParams:   "RTX 4090",
		InstallerArgs: "Studio",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'NvidiaDriver.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading Nvidia driver...\"\nInvoke-WebRequest -Uri 'https://mock.nvidia.download/driver.exe' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop\nWrite-Host \"Installing Nvidia driver silently...\"\n$p = Start-Process -FilePath $tempExe -ArgumentList '-s' -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1) {\n    throw \"Nvidia driver installation failed with exit code \" + $p.ExitCode\n}\n"): {},
		},
	}

	issues := InstallPackages(context.Background(), runner, []Package{pkg})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %v", issues)
	}

	if !runner.called("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'NvidiaDriver.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading Nvidia driver...\"\nInvoke-WebRequest -Uri 'https://mock.nvidia.download/driver.exe' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop\nWrite-Host \"Installing Nvidia driver silently...\"\n$p = Start-Process -FilePath $tempExe -ArgumentList '-s' -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1) {\n    throw \"Nvidia driver installation failed with exit code \" + $p.ExitCode\n}\n") {
		t.Error("expected Nvidia installer script to be executed")
	}
}

func TestCheckPackagesNvidiaBroadcast(t *testing.T) {
	pkg := Package{
		Name: "Nvidia Broadcast",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", `if (Test-Path "${env:ProgramFiles}\NVIDIA Corporation\NVIDIA Broadcast\NVIDIA Broadcast.exe") { 'yes' } else { 'no' }`): {
				Stdout: "yes\n",
			},
		},
	}

	states := CheckPackages(context.Background(), runner, []Package{pkg}, 1)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if !states[0].Installed {
		t.Error("expected Nvidia Broadcast to be reported as installed")
	}
}

func TestInstallPackagesNvidiaBroadcast(t *testing.T) {
	pkg := Package{
		Name: "Nvidia Broadcast",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'NvidiaBroadcast.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading Nvidia Broadcast...\"\nInvoke-WebRequest -Uri 'https://international.download.nvidia.com/Windows/broadcast/2.2.0/NVIDIA_Broadcast_v2.2.0.52896077.exe' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop\nWrite-Host \"Installing Nvidia Broadcast silently...\"\n$p = Start-Process -FilePath $tempExe -ArgumentList '/s' -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {\n    throw \"Nvidia Broadcast installation failed with exit code \" + $p.ExitCode\n}\n"): {},
		},
	}

	issues := InstallPackages(context.Background(), runner, []Package{pkg})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %v", issues)
	}

	if !runner.called("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'NvidiaBroadcast.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading Nvidia Broadcast...\"\nInvoke-WebRequest -Uri 'https://international.download.nvidia.com/Windows/broadcast/2.2.0/NVIDIA_Broadcast_v2.2.0.52896077.exe' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop\nWrite-Host \"Installing Nvidia Broadcast silently...\"\n$p = Start-Process -FilePath $tempExe -ArgumentList '/s' -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {\n    throw \"Nvidia Broadcast installation failed with exit code \" + $p.ExitCode\n}\n") {
		t.Error("expected Nvidia Broadcast installer script to be executed")
	}
}

func TestCheckPackagesAmdAutoDetect(t *testing.T) {
	pkg := Package{
		Name: "AMD Auto-Detect",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", `if (Test-Path "${env:ProgramFiles}\AMD\CNext\CNext\RadeonSoftware.exe" -or Test-Path "${env:ProgramFiles}\AMD\CNext\CNext\AMDRSServ.exe") { 'yes' } else { 'no' }`): {
				Stdout: "yes\n",
			},
		},
	}

	states := CheckPackages(context.Background(), runner, []Package{pkg}, 1)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if !states[0].Installed {
		t.Error("expected AMD Auto-Detect to be reported as installed")
	}
}

func TestInstallPackagesAmdAutoDetect(t *testing.T) {
	pkg := Package{
		Name: "AMD Auto-Detect",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'AMDMinimalSetup.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading AMD Auto-Detect tool...\"\nInvoke-WebRequest -Uri 'https://drivers.amd.com/drivers/installer/26.10/whql/amd-software-adrenalin-edition-26.6.1-minimalsetup-260601_web.exe' -OutFile $tempExe -UseBasicParsing -Headers @{\"Referer\" = \"https://www.amd.com/\"} -UserAgent \"Mozilla/5.0 (Windows NT 10.0; Win64; x64)\" -ErrorAction Stop\nWrite-Host \"Launching AMD Auto-Detect tool...\"\n$p = Start-Process -FilePath $tempExe -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {\n    throw \"AMD Auto-Detect failed with exit code \" + $p.ExitCode\n}\n"): {},
		},
	}

	issues := InstallPackages(context.Background(), runner, []Package{pkg})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %v", issues)
	}

	if !runner.called("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'AMDMinimalSetup.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading AMD Auto-Detect tool...\"\nInvoke-WebRequest -Uri 'https://drivers.amd.com/drivers/installer/26.10/whql/amd-software-adrenalin-edition-26.6.1-minimalsetup-260601_web.exe' -OutFile $tempExe -UseBasicParsing -Headers @{\"Referer\" = \"https://www.amd.com/\"} -UserAgent \"Mozilla/5.0 (Windows NT 10.0; Win64; x64)\" -ErrorAction Stop\nWrite-Host \"Launching AMD Auto-Detect tool...\"\n$p = Start-Process -FilePath $tempExe -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {\n    throw \"AMD Auto-Detect failed with exit code \" + $p.ExitCode\n}\n") {
		t.Error("expected AMD Auto-Detect installer script to be executed")
	}
}

func TestCheckPackagesIntelDSA(t *testing.T) {
	pkg := Package{
		Name: "Intel DSA",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", `if (Test-Path "${env:ProgramFiles(x86)}\Intel\Driver and Support Assistant\DSAService.exe") { 'yes' } else { 'no' }`): {
				Stdout: "yes\n",
			},
		},
	}

	states := CheckPackages(context.Background(), runner, []Package{pkg}, 1)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if !states[0].Installed {
		t.Error("expected Intel DSA to be reported as installed")
	}
}

func TestInstallPackagesIntelDSA(t *testing.T) {
	pkg := Package{
		Name: "Intel DSA",
	}

	runner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'IntelDSAInstaller.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading Intel Driver & Support Assistant...\"\nInvoke-WebRequest -Uri 'https://dsadata.intel.com/installer' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop\nWrite-Host \"Installing Intel Driver & Support Assistant silently...\"\n$p = Start-Process -FilePath $tempExe -ArgumentList '-s', '-norestart' -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {\n    throw \"Intel DSA installation failed with exit code \" + $p.ExitCode\n}\n"): {},
		},
	}

	issues := InstallPackages(context.Background(), runner, []Package{pkg})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %v", issues)
	}

	if !runner.called("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "\n$tempExe = Join-Path $env:TEMP 'IntelDSAInstaller.exe'\nif (Test-Path $tempExe) { Remove-Item -Force $tempExe }\n[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072\nWrite-Host \"Downloading Intel Driver & Support Assistant...\"\nInvoke-WebRequest -Uri 'https://dsadata.intel.com/installer' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop\nWrite-Host \"Installing Intel Driver & Support Assistant silently...\"\n$p = Start-Process -FilePath $tempExe -ArgumentList '-s', '-norestart' -Wait -PassThru\nRemove-Item -Force $tempExe\nif ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {\n    throw \"Intel DSA installation failed with exit code \" + $p.ExitCode\n}\n") {
		t.Error("expected Intel DSA installer script to be executed")
	}
}

