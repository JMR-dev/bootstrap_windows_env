package bootstrap

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type WSLState struct {
	Installed       bool
	Version2Default bool
	RestartRequired bool
	FedoraAvailable string
	FedoraInstalled string
	FedoraReady     bool
	GuestConfigured bool
}

type WSLTransition string

const (
	WSLInstallPrerequisites WSLTransition = "install-prerequisites"
	WSLWaitForRestart       WSLTransition = "wait-for-restart"
	WSLSetDefaultVersion    WSLTransition = "set-default-version"
	WSLInstallFedora        WSLTransition = "install-fedora"
	WSLWaitForFedoraUser    WSLTransition = "wait-for-fedora-user"
	WSLRunGuestBootstrap    WSLTransition = "run-guest-bootstrap"
	WSLComplete             WSLTransition = "complete"
)

func NextWSLTransition(state WSLState) WSLTransition {
	switch {
	case !state.Installed:
		return WSLInstallPrerequisites
	case state.RestartRequired:
		return WSLWaitForRestart
	case !state.Version2Default:
		return WSLSetDefaultVersion
	case state.FedoraInstalled == "":
		return WSLInstallFedora
	case !state.FedoraReady:
		return WSLWaitForFedoraUser
	case !state.GuestConfigured:
		return WSLRunGuestBootstrap
	default:
		return WSLComplete
	}
}

func DetectWSL(ctx context.Context, runner Runner) (WSLState, error) {
	var state WSLState
	status := runner.Run(ctx, "wsl.exe", "--status")
	if status.Err != nil {
		return state, nil
	}
	state.Installed = true
	state.Version2Default = strings.Contains(strings.ToLower(status.CombinedOutput()), "default version: 2")
	restart := runner.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		`if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { 'true' } else { 'false' }`)
	state.RestartRequired = strings.Contains(strings.ToLower(restart.Stdout), "true")

	online := runner.Run(ctx, "wsl.exe", "--list", "--online")
	if online.Err != nil {
		return state, fmt.Errorf("list online WSL distributions: %w: %s", online.Err, online.CombinedOutput())
	}
	state.FedoraAvailable = SelectOfficialFedora(online.Stdout)
	if state.FedoraAvailable == "" {
		return state, fmt.Errorf("no official FedoraLinux-* distribution was reported by wsl --list --online")
	}
	installed := runner.Run(ctx, "wsl.exe", "--list", "--quiet")
	if installed.Err != nil {
		return state, fmt.Errorf("list installed WSL distributions: %w: %s", installed.Err, installed.CombinedOutput())
	}
	for _, line := range strings.Split(strings.ReplaceAll(installed.Stdout, "\r", ""), "\n") {
		distro := strings.TrimSpace(strings.ReplaceAll(line, "\x00", ""))
		if strings.HasPrefix(distro, "FedoraLinux-") {
			state.FedoraInstalled = distro
			break
		}
	}
	if state.FedoraInstalled == "" {
		return state, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ready := runner.Run(probeCtx, "wsl.exe", "-d", state.FedoraInstalled, "--exec", "sh", "-lc", "id -u >/dev/null")
	state.FedoraReady = ready.Err == nil
	if state.FedoraReady {
		marker := runner.Run(ctx, "wsl.exe", "-d", state.FedoraInstalled, "--exec", "sh", "-lc", "test -f ~/.local/state/bootstrap_dev_env/wsl-complete")
		state.GuestConfigured = marker.Err == nil
	}
	return state, nil
}

var fedoraPattern = regexp.MustCompile(`(?i)\bFedoraLinux-(\d+)\b`)

func SelectOfficialFedora(output string) string {
	matches := fedoraPattern.FindAllStringSubmatch(output, -1)
	type release struct {
		name    string
		version int
	}
	var releases []release
	for _, match := range matches {
		version, _ := strconv.Atoi(match[1])
		releases = append(releases, release{name: match[0], version: version})
	}
	sort.Slice(releases, func(i, j int) bool { return releases[i].version > releases[j].version })
	if len(releases) == 0 {
		return ""
	}
	return releases[0].name
}

type WSLResult struct {
	Notice string
	Issue  error
	Done   bool
}

func RunWSLPhase(ctx context.Context, runner Runner, opts Options) WSLResult {
	state, err := DetectWSL(ctx, runner)
	if err != nil {
		return WSLResult{Issue: err}
	}
	switch NextWSLTransition(state) {
	case WSLInstallPrerequisites:
		result := runner.Run(ctx, "wsl.exe", "--install", "--no-distribution")
		if result.Err != nil {
			return WSLResult{Issue: fmt.Errorf("install WSL prerequisites: %w: %s", result.Err, result.CombinedOutput())}
		}
		return WSLResult{Notice: "WSL prerequisites were installed. Restart Windows, then rerun this bootstrapper to install Fedora."}
	case WSLWaitForRestart:
		return WSLResult{Notice: "Windows has a pending restart. Restart Windows, then rerun this bootstrapper to resume Fedora WSL setup."}
	case WSLSetDefaultVersion:
		result := runner.Run(ctx, "wsl.exe", "--set-default-version", "2")
		if result.Err != nil {
			return WSLResult{Issue: fmt.Errorf("set WSL default version to 2: %w: %s", result.Err, result.CombinedOutput())}
		}
		return WSLResult{Notice: "WSL default version was set to 2. Rerun this bootstrapper to continue Fedora setup."}
	case WSLInstallFedora:
		result := runner.Run(ctx, "wsl.exe", "--install", "--distribution", state.FedoraAvailable, "--no-launch")
		if result.Err != nil {
			return WSLResult{Issue: fmt.Errorf("install %s: %w: %s", state.FedoraAvailable, result.Err, result.CombinedOutput())}
		}
		return WSLResult{Notice: fmt.Sprintf("%s was installed. Run `wsl -d %s` once to create its Linux username, then rerun this bootstrapper.", state.FedoraAvailable, state.FedoraAvailable)}
	case WSLWaitForFedoraUser:
		return WSLResult{Notice: fmt.Sprintf("Fedora requires first-launch setup. Run `wsl -d %s` and create its Linux username, then rerun this bootstrapper.", state.FedoraInstalled)}
	case WSLRunGuestBootstrap:
		invocation := GuestBootstrapInvocation(opts, state.FedoraInstalled)
		result := runner.Run(ctx, invocation.Name, invocation.Args...)
		if result.Err != nil {
			return WSLResult{Issue: fmt.Errorf("run Fedora bootstrap: %w: %s", result.Err, result.CombinedOutput())}
		}
		return WSLResult{Notice: "Fedora WSL guest provisioning completed.", Done: true}
	default:
		return WSLResult{Notice: "Fedora WSL guest was already provisioned.", Done: true}
	}
}

func GuestBootstrapInvocation(opts Options, distro string) CommandSpec {
	arguments := []string{"--wsl", "--headless", "--yes"}
	if opts.NoAI {
		arguments = append(arguments, "--no-ai")
	}
	script := `set -eu
repo="$1"
shift
arch="$(uname -m)"
case "$arch" in
  x86_64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *) echo "Unsupported guest architecture: $arch" >&2; exit 1 ;;
esac
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" -o "$work/release.json"
python3 - "$work/release.json" "$asset_arch" > "$work/assets" <<'PY'
import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
arch = sys.argv[2]
assets = data.get("assets", [])
binaries = [a for a in assets if "linux" in a["name"].lower() and arch in a["name"].lower() and "sha256" not in a["name"].lower() and "checksum" not in a["name"].lower()]
if not binaries:
    raise SystemExit("No Linux bootstrap release asset found for " + arch)
binary = binaries[0]
print(binary["browser_download_url"])
checksums = [a for a in assets if "sha256" in a["name"].lower() or "checksum" in a["name"].lower()]
print(checksums[0]["browser_download_url"] if checksums else "")
PY
binary_url="$(sed -n '1p' "$work/assets")"
checksum_url="$(sed -n '2p' "$work/assets")"
curl -fsSL "$binary_url" -o "$work/bootstrap_dev_env"
chmod +x "$work/bootstrap_dev_env"
if [ -n "$checksum_url" ]; then
  curl -fsSL "$checksum_url" -o "$work/checksums"
  expected="$(grep "$(basename "$binary_url")" "$work/checksums" | awk '{print $1}' | head -n 1)"
  [ -n "$expected" ] || { echo "Published checksum did not contain the selected asset" >&2; exit 1; }
  actual="$(sha256sum "$work/bootstrap_dev_env" | awk '{print $1}')"
  [ "$expected" = "$actual" ] || { echo "Linux bootstrap checksum verification failed" >&2; exit 1; }
fi
"$work/bootstrap_dev_env" "$@"
mkdir -p "$HOME/.local/state/bootstrap_dev_env"
touch "$HOME/.local/state/bootstrap_dev_env/wsl-complete"`
	args := []string{"-d", distro, "--exec", "sh", "-lc", script, "bootstrap-wsl", opts.LinuxReleaseRepo}
	args = append(args, arguments...)
	return CommandSpec{Name: "wsl.exe", Args: args}
}
