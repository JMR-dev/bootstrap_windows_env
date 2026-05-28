# bootstrap_windows_env

Windows-native bootstrapper for a personal Windows 11 development workstation, with an optional Fedora WSL handoff.

The tool is intentionally conservative:

- Windows packages come from `winget` by exact package ID whenever available.
- Secondary managed providers such as Scoop or Chocolatey are used only for items without a reliable `winget` manifest, such as `pyenv-win`.
- Unsupported packages are reported as deferred instead of installed through ad hoc custom installers.
- WSL guest provisioning expects the published `JMR-dev/bootstrap_dev_env` Linux release to support `--wsl`; this repository does not modify that Linux project.

## Build

```powershell
go build ./cmd/bootstrap_windows_env
```

Tagged GitHub releases build and upload a precompiled `bootstrap_windows_env.exe`.

## Usage

```powershell
.\bootstrap_windows_env.exe
.\bootstrap_windows_env.exe --yes
.\bootstrap_windows_env.exe --dry-run
.\bootstrap_windows_env.exe --only os
.\bootstrap_windows_env.exe --only host
.\bootstrap_windows_env.exe --only custom
.\bootstrap_windows_env.exe --only config
.\bootstrap_windows_env.exe --only wsl
.\bootstrap_windows_env.exe --no-ai
.\bootstrap_windows_env.exe --no-wsl
.\bootstrap_windows_env.exe --no-restore-points
```

`--no-restore-points` skips the two System Restore checkpoints in the `os` phase. Use this on Windows Server SKUs (including GitHub Actions `windows-latest` runners), where `Checkpoint-Computer` is not supported.

## Phases

`os` runs before any application installation. It invokes Windows Update/Microsoft Update, stops for reboot when required, creates a restore point, runs Chris Titus Tech WinUtil with the managed sane-default config, enables Developer Mode and Windows sudo, applies power/update/UI/store/taskbar defaults, then creates a second restore point before package installation.

`host` installs exact `winget` packages, configures Hyper-V when supported for Vagrant, provisions Node.js LTS through `fnm`, installs `pyenv-win` through Scoop or Chocolatey, provisions the latest stable Python through `pyenv`, and imports VLC default media-player associations for common audio/video file types.

Visual Studio Community 2026 is installed as `Microsoft.VisualStudio.Community` with the managed `.vsconfig` exported from the current workstation. That config includes the native desktop workload, Microsoft C/C++ tools, Windows SDKs, and CMake integration. The Windows host intentionally does not install `gcc` or `g++`; native C and C++ builds should use the Visual Studio toolchain.

`custom` installs `oh-my-posh`, `agy` through Google's official Windows PowerShell installer, Node-based AI CLIs unless `--no-ai` is set, Playwright browsers, and the `JMR-dev/gh-repo-bootstrap` GitHub CLI extension.

`config` deploys managed WezTerm, PowerShell profile, and `oh-my-posh` assets, backs up differing existing files with numbered suffixes, clones `JMR-dev/nvim-config`, and optionally offers interactive GitHub/SSH setup.

`wsl` enables WSL prerequisites, installs the current official `FedoraLinux-*` distro reported by `wsl --list --online`, stops for reboot or first-launch username creation when needed, then invokes the latest Linux bootstrap release with `--wsl --headless --yes`.

## Current Deferrals

`Webcamoid` is deferred because no exact `winget` manifest was available during implementation. Native Docker Desktop, Podman Desktop, Buildah, Minikube, Firecracker, QEMU, virt-manager, Windows Terminal, and VS Code are also intentionally excluded from Windows v1.
