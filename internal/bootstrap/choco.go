package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var chocoCommand = "choco"

func init() {
	if _, err := exec.LookPath("choco"); err == nil {
		chocoCommand = "choco"
		return
	}
	if chocoInstall := os.Getenv("ChocolateyInstall"); chocoInstall != "" {
		path := filepath.Join(chocoInstall, "bin", "choco.exe")
		if _, err := os.Stat(path); err == nil {
			chocoCommand = path
			return
		}
	}
	defaultPath := `C:\ProgramData\chocolatey\bin\choco.exe`
	if _, err := os.Stat(defaultPath); err == nil {
		chocoCommand = defaultPath
	}
}

func ChocoInstallArgs(pkg Package) []string {
	args := []string{
		"install", pkg.ChocoID, "-y", "--no-progress",
	}
	if pkg.ChocoParams != "" {
		args = append(args, "--package-parameters", pkg.ChocoParams)
	}
	if pkg.InstallerArgs != "" {
		args = append(args, "--install-arguments", pkg.InstallerArgs)
	}
	return args
}

func ChocoListArgs(pkg Package) []string {
	return []string{
		"list", "--local-only", "--exact", pkg.ChocoID,
	}
}

func ChocoOutputShowsInstalled(pkg Package, output string) bool {
	lower := strings.ToLower(output)
	lines := strings.Split(lower, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, strings.ToLower(pkg.ChocoID)+" ") {
			return true
		}
	}
	return false
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
				state := PackageState{Package: current.pkg}
				if isCustomPackage(current.pkg) {
					cmd := ""
					switch current.pkg.Name {
					case "FilePilot":
						cmd = "if (Get-Command filepilot.exe -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }"
					case "Affinity":
						cmd = `if (Get-AppxPackage *Affinity* -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }`
					case "Tidal":
						cmd = `if (Test-Path "${env:LocalAppData}\TIDAL\TIDAL.exe" -or Test-Path "${env:ProgramFiles}\TIDAL\TIDAL.exe") { 'yes' } else { 'no' }`
					case "DaVinci Resolve":
						cmd = `if (Test-Path "${env:ProgramFiles}\Blackmagic Design\DaVinci Resolve\Resolve.exe") { 'yes' } else { 'no' }`
					case "WhatsApp":
						cmd = `if (Get-AppxPackage *WhatsApp* -ErrorAction SilentlyContinue) { 'yes' } else { 'no' }`
					case "Ardour":
						cmd = `if (Test-Path "C:\msys64\mingw64\bin\ardour8.exe" -or Test-Path "C:\msys64\mingw64\bin\ardour.exe" -or Test-Path "C:\msys64\mingw64\bin\ardour9.exe" -or Test-Path "C:\tools\msys64\mingw64\bin\ardour8.exe" -or Test-Path "C:\tools\msys64\mingw64\bin\ardour.exe" -or Test-Path "C:\tools\msys64\mingw64\bin\ardour9.exe") { 'yes' } else { 'no' }`
					case "Nvidia Broadcast":
						cmd = `if (Test-Path "${env:ProgramFiles}\NVIDIA Corporation\NVIDIA Broadcast\NVIDIA Broadcast.exe") { 'yes' } else { 'no' }`
					case "AMD Auto-Detect":
						cmd = `if (Test-Path "${env:ProgramFiles}\AMD\CNext\CNext\RadeonSoftware.exe" -or Test-Path "${env:ProgramFiles}\AMD\CNext\CNext\AMDRSServ.exe") { 'yes' } else { 'no' }`
					case "Intel DSA":
						cmd = `if (Test-Path "${env:ProgramFiles(x86)}\Intel\Driver and Support Assistant\DSAService.exe") { 'yes' } else { 'no' }`
					case "Nvidia Driver":
						model := current.pkg.ChocoParams
						driverType := current.pkg.InstallerArgs
						pfid, err := resolveNvidiaPFID(model)
						if err != nil {
							state.CheckErr = err
							state.Installed = false
						} else {
							_, apiVer, err := fetchNvidiaDriverURLFn(pfid, driverType)
							if err != nil {
								state.CheckErr = err
								state.Installed = false
							} else {
								installedVer, err := getInstalledNvidiaVersionFn(ctx, runner)
								if err != nil {
									state.CheckErr = err
									state.Installed = false
								} else {
									state.Installed = (installedVer != "" && installedVer == apiVer)
								}
							}
						}
					}
					if cmd != "" {
						result := runner.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", cmd)
						state.Installed = strings.Contains(strings.ToLower(result.CombinedOutput()), "yes")
					} else if current.pkg.Name != "Nvidia Driver" {
						state.Installed = false
					}
				} else {
					result := runner.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Test-Path C:\\ProgramData\\chocolatey\\lib\\"+current.pkg.ChocoID)
					if result.Err != nil {
						state.CheckErr = result.Err
					} else {
						state.Installed = strings.Contains(strings.ToLower(result.CombinedOutput()), "true")
					}
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
		pkgTimeout := 5 * time.Minute
		if pkg.Name == "Ardour" || pkg.Name == "Nvidia Driver" || pkg.Name == "Nvidia Broadcast" || pkg.Name == "AMD Auto-Detect" || pkg.Name == "Intel DSA" {
			pkgTimeout = 30 * time.Minute
		} else if pkg.Name == "DaVinci Resolve" {
			pkgTimeout = 15 * time.Minute
		}
		if IsSessionZero() && pkg.Class == ClassGUI {
			if pkg.Name != "Nvidia Driver" && pkg.Name != "Nvidia Broadcast" && pkg.Name != "AMD Auto-Detect" && pkg.Name != "Intel DSA" {
				pkgTimeout = 10 * time.Second
			}
		}
		
		pkgCtx, pkgCancel := context.WithTimeout(ctx, pkgTimeout)
		
		if isCustomPackage(pkg) {
			var script string
			switch pkg.Name {
			case "FilePilot":
				script = `$tempExe = Join-Path $env:TEMP 'FilePilotSetup.exe'; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; Invoke-WebRequest -Uri 'https://filepilot.tech/download/latest' -OutFile $tempExe -UseBasicParsing; Start-Process -FilePath $tempExe -ArgumentList '/S' -Wait`
			case "Affinity":
				script = `
$tempMsix = Join-Path $env:TEMP 'Affinity_x64.msix'
if (Test-Path $tempMsix) { Remove-Item -Force $tempMsix }
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Write-Host "Downloading Affinity MSIX..."
Invoke-WebRequest -Uri 'https://downloads.affinity.studio/Affinity%20x64.msix' -OutFile $tempMsix -UseBasicParsing -ErrorAction Stop
Write-Host "Installing Affinity MSIX package..."
Add-AppxProvisionedPackage -Online -PackagePath $tempMsix -SkipLicense
`
			case "Tidal":
				script = `
$tempExe = Join-Path $env:TEMP 'TIDALSetup.exe'
if (Test-Path $tempExe) { Remove-Item -Force $tempExe }
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Write-Host "Downloading Tidal setup..."
Invoke-WebRequest -Uri 'https://download.tidal.com/desktop/TIDALSetup.exe' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop
Write-Host "Installing Tidal silently..."
Start-Process -FilePath $tempExe -ArgumentList '-s' -Wait
`
			case "WhatsApp":
				script = `winget install --id WhatsApp.WhatsApp --silent --accept-package-agreements --accept-source-agreements`
			case "DaVinci Resolve":
				script = `winget install --id BlackmagicDesign.DaVinciResolve --silent --accept-package-agreements --accept-source-agreements`
			case "Ardour":
				script = `
$msysDir = "C:\msys64"
if (-not (Test-Path "$msysDir\usr\bin\bash.exe")) {
    $msysDir = "C:\tools\msys64"
}
if (-not (Test-Path "$msysDir\usr\bin\bash.exe")) {
    Write-Host "Installing MSYS2 via Chocolatey..."
    choco install msys2 -y --no-progress
    $msysDir = "C:\tools\msys64"
    if (-not (Test-Path "$msysDir\usr\bin\bash.exe")) {
        $msysDir = "C:\msys64"
    }
}
$env:MSYSTEM="MINGW64"
$buildScript = @'
set -e
pacman -Sy --noconfirm
pacman -S --noconfirm --needed git unzip make cmake mingw-w64-x86_64-toolchain mingw-w64-x86_64-boost mingw-w64-x86_64-glib2 mingw-w64-x86_64-glibmm mingw-w64-x86_64-libsndfile mingw-w64-x86_64-curl mingw-w64-x86_64-libarchive mingw-w64-x86_64-liblo mingw-w64-x86_64-taglib mingw-w64-x86_64-vamp-plugin-sdk mingw-w64-x86_64-rubberband mingw-w64-x86_64-aubio mingw-w64-x86_64-cairomm mingw-w64-x86_64-pangomm mingw-w64-x86_64-jack2 mingw-w64-x86_64-serd mingw-w64-x86_64-sord mingw-w64-x86_64-sratom mingw-w64-x86_64-lilv mingw-w64-x86_64-libwebsockets mingw-w64-x86_64-libusb python3
if [ ! -d "/tmp/ardour" ]; then
    git clone --depth 1 https://github.com/Ardour/ardour.git /tmp/ardour
fi
cd /tmp/ardour
python3 ./waf configure --prefix=/mingw64 --dist-target=mingw --optimize
python3 ./waf build
python3 ./waf install
'@
$scriptPath = "$msysDir\tmp\build_ardour.sh"
[System.IO.Directory]::CreateDirectory("$msysDir\tmp") | Out-Null
[System.IO.File]::WriteAllText($scriptPath, $buildScript)
& "$msysDir\usr\bin\bash.exe" --login /tmp/build_ardour.sh
`
			case "Nvidia Driver":
				model := pkg.ChocoParams
				driverType := pkg.InstallerArgs
				pfid, err := resolveNvidiaPFID(model)
				if err != nil {
					issues = append(issues, Issue{
						Step: "resolve Nvidia model",
						Err:  err,
					})
				} else {
					downloadURL, _, err := fetchNvidiaDriverURLFn(pfid, driverType)
					if err != nil {
						issues = append(issues, Issue{
							Step: "fetch Nvidia driver URL",
							Err:  err,
						})
					} else {
						script = fmt.Sprintf(`
$tempExe = Join-Path $env:TEMP 'NvidiaDriver.exe'
if (Test-Path $tempExe) { Remove-Item -Force $tempExe }
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Write-Host "Downloading Nvidia driver..."
Invoke-WebRequest -Uri '%s' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop
Write-Host "Installing Nvidia driver silently..."
$p = Start-Process -FilePath $tempExe -ArgumentList '-s' -Wait -PassThru
Remove-Item -Force $tempExe
if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1) {
    throw "Nvidia driver installation failed with exit code " + $p.ExitCode
}
`, downloadURL)
					}
				}
			case "Nvidia Broadcast":
				downloadURL := "https://international.download.nvidia.com/Windows/broadcast/2.2.0/NVIDIA_Broadcast_v2.2.0.52896077.exe"
				script = fmt.Sprintf(`
$tempExe = Join-Path $env:TEMP 'NvidiaBroadcast.exe'
if (Test-Path $tempExe) { Remove-Item -Force $tempExe }
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Write-Host "Downloading Nvidia Broadcast..."
Invoke-WebRequest -Uri '%s' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop
Write-Host "Installing Nvidia Broadcast silently..."
$p = Start-Process -FilePath $tempExe -ArgumentList '/s' -Wait -PassThru
Remove-Item -Force $tempExe
if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {
    throw "Nvidia Broadcast installation failed with exit code " + $p.ExitCode
}
`, downloadURL)
			case "AMD Auto-Detect":
				downloadURL := "https://drivers.amd.com/drivers/installer/26.10/whql/amd-software-adrenalin-edition-26.6.1-minimalsetup-260601_web.exe"
				script = fmt.Sprintf(`
$tempExe = Join-Path $env:TEMP 'AMDMinimalSetup.exe'
if (Test-Path $tempExe) { Remove-Item -Force $tempExe }
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Write-Host "Downloading AMD Auto-Detect tool..."
Invoke-WebRequest -Uri '%s' -OutFile $tempExe -UseBasicParsing -Headers @{"Referer" = "https://www.amd.com/"} -UserAgent "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" -ErrorAction Stop
Write-Host "Launching AMD Auto-Detect tool..."
$p = Start-Process -FilePath $tempExe -Wait -PassThru
Remove-Item -Force $tempExe
if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {
    throw "AMD Auto-Detect failed with exit code " + $p.ExitCode
}
`, downloadURL)
			case "Intel DSA":
				downloadURL := "https://dsadata.intel.com/installer"
				script = fmt.Sprintf(`
$tempExe = Join-Path $env:TEMP 'IntelDSAInstaller.exe'
if (Test-Path $tempExe) { Remove-Item -Force $tempExe }
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
Write-Host "Downloading Intel Driver & Support Assistant..."
Invoke-WebRequest -Uri '%s' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop
Write-Host "Installing Intel Driver & Support Assistant silently..."
$p = Start-Process -FilePath $tempExe -ArgumentList '-s', '-norestart' -Wait -PassThru
Remove-Item -Force $tempExe
if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1 -and $p.ExitCode -ne 3010) {
    throw "Intel DSA installation failed with exit code " + $p.ExitCode
}
`, downloadURL)
			}
			if script != "" {
				result := runner.Run(pkgCtx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
				if result.Err != nil {
					issues = append(issues, Issue{
						Step: "install " + pkg.Name + " (custom download/install)",
						Err:  fmt.Errorf("%w: %s", result.Err, result.CombinedOutput()),
					})
				}
			}
			pkgCancel()
			continue
		}

		prepared, err := PreparePackageForInstall(pkg)
		if err != nil {
			issues = append(issues, Issue{Step: "prepare " + pkg.Name, Err: err})
			pkgCancel()
			continue
		}
		result := runner.Run(pkgCtx, chocoCommand, ChocoInstallArgs(prepared)...)
		if result.Err != nil {
			isSuccess := false
			var exitErr *exec.ExitError
			if errors.As(result.Err, &exitErr) {
				code := exitErr.ExitCode()
				if code == 3010 || code == 1641 {
					isSuccess = true
				}
			}
			if !isSuccess {
				out := result.CombinedOutput()
				if len(out) > 500 {
					out = out[:500] + "... (truncated)"
				}
				issues = append(issues, Issue{
					Step: "install " + pkg.Name,
					Err:  fmt.Errorf("%w: %s", result.Err, out),
				})
			}
		}
		pkgCancel()
	}
	return issues
}

func PreparePackageForInstall(pkg Package) (Package, error) {
	if pkg.ChocoConfigAsset == "" {
		return pkg, nil
	}
	path, err := writeInstallAsset(pkg.ChocoConfigAsset)
	if err != nil {
		return pkg, err
	}
	pkg.InstallerArgs = strings.ReplaceAll(pkg.InstallerArgs, "{config}", `"`+path+`"`)
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

func isCustomPackage(pkg Package) bool {
	return pkg.ChocoID == "" && (pkg.Name == "FilePilot" || pkg.Name == "Affinity" || pkg.Name == "Tidal" || pkg.Name == "DaVinci Resolve" || pkg.Name == "Ardour" || pkg.Name == "WhatsApp" || pkg.Name == "Nvidia Driver" || pkg.Name == "Nvidia Broadcast" || pkg.Name == "AMD Auto-Detect" || pkg.Name == "Intel DSA")
}

var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procProcessIdToSessionId = modkernel32.NewProc("ProcessIdToSessionId")
	procGetCurrentProcessId  = modkernel32.NewProc("GetCurrentProcessId")
)

func IsSessionZero() bool {
	pid, _, _ := procGetCurrentProcessId.Call()
	var sessionID uint32
	r, _, _ := procProcessIdToSessionId.Call(pid, uintptr(unsafe.Pointer(&sessionID)))
	if r == 0 {
		return false
	}
	return sessionID == 0
}
