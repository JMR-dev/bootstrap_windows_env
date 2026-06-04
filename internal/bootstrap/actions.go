package bootstrap

import (
	"context"
	"fmt"
	"strings"
)

type CommandSpec struct {
	Name string
	Args []string
}

type Action struct {
	Name     string
	Phase    Phase
	Probe    CommandSpec
	Commands []CommandSpec
	AI       bool
}

const refreshPath = `$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')+';C:\ProgramData\chocolatey\bin'; if (Get-Command fnm -ErrorAction SilentlyContinue) { fnm env --shell powershell | Invoke-Expression }; `

func powerShell(script string) CommandSpec {
	return CommandSpec{
		Name: "powershell.exe",
		Args: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", refreshPath + script},
	}
}

func checkedNativePowerShell(command string) CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'; ` + command + `; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }`)
}

func HostActions(opts Options) []Action {
	actions := []Action{
		{
			Name:  "Visual Studio Community 2026",
			Phase: PhaseHost,
			Probe: powerShell(`if (Test-Path "${env:ProgramFiles}\Microsoft Visual Studio\2026\Community\Common7\IDE\devenv.exe") { exit 0 } else { exit 1 }`),
			Commands: []CommandSpec{
				powerShell(fmt.Sprintf(
					`$tempExe = Join-Path $env:TEMP 'vs_community.exe'; `+
						`if (Test-Path $tempExe) { Remove-Item -Force $tempExe }; `+
						`[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; `+
						`Write-Host 'Downloading Visual Studio 2026 bootstrapper...'; `+
						`Invoke-WebRequest -Uri 'https://aka.ms/vs/17/release/vs_community.exe' -OutFile $tempExe -UseBasicParsing -ErrorAction Stop; `+
						`Unblock-File -Path $tempExe; `+
						`Write-Host 'Installing Visual Studio 2026 with workloads...'; `+
						`$p = Start-Process -FilePath $tempExe -ArgumentList '--passive --wait --config "%s"' -Wait -PassThru; `+
						`if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 3010) { throw 'Visual Studio installation failed with exit code ' + $p.ExitCode }`,
					mustInstallAssetPath("assets/visual-studio-community.vsconfig"),
				)),
			},
		},
		{
			Name:  "fnm and Node.js LTS",
			Phase: PhaseHost,
			Probe: powerShell(`fnm exec --using=lts-latest node --version | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				powerShell(`$ErrorActionPreference='Stop'
function Invoke-Native {
    param([Parameter(Mandatory=$true)][string]$File, [Parameter(Mandatory=$true)][string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}
$scoop = Get-Command scoop -ErrorAction SilentlyContinue
$scoopShim = Join-Path $HOME 'scoop\shims\scoop.ps1'
if ($scoop) {
    Invoke-Native $scoop.Source @('install', 'fnm')
} elseif (Test-Path $scoopShim) {
    Invoke-Native $scoopShim @('install', 'fnm')
} elseif (Get-Command choco -ErrorAction SilentlyContinue) {
    Invoke-Native 'choco' @('install', 'fnm', '-y', '--no-progress')
} else {
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    Invoke-RestMethod -Uri 'https://community.chocolatey.org/install.ps1' | Invoke-Expression
    $env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
    Invoke-Native 'choco' @('install', 'fnm', '-y', '--no-progress')
}
$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
Invoke-Native 'fnm' @('install', '--lts')
Invoke-Native 'fnm' @('default', 'lts-latest')`),
			},
		},
		{
			Name:  "pyenv-win through Scoop or Chocolatey",
			Phase: PhaseHost,
			Probe: powerShell(`pyenv --version | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				powerShell(`$ErrorActionPreference='Stop'
function Invoke-Native {
    param([Parameter(Mandatory=$true)][string]$File, [Parameter(Mandatory=$true)][string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}
$scoop = Get-Command scoop -ErrorAction SilentlyContinue
$scoopShim = Join-Path $HOME 'scoop\shims\scoop.ps1'
if ($scoop) {
    Invoke-Native $scoop.Source @('install', 'pyenv')
} elseif (Test-Path $scoopShim) {
    Invoke-Native $scoopShim @('install', 'pyenv')
} elseif (Get-Command choco -ErrorAction SilentlyContinue) {
    Invoke-Native 'choco' @('install', 'pyenv-win', '-y', '--no-progress')
} else {
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    Invoke-RestMethod -Uri 'https://community.chocolatey.org/install.ps1' | Invoke-Expression
    $env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
    Invoke-Native 'choco' @('install', 'pyenv-win', '-y', '--no-progress')
}`),
			},
		},
		{
			Name:  "latest stable Python through pyenv-win",
			Phase: PhaseHost,
			Probe: powerShell(`$v = pyenv version-name; if ([string]::IsNullOrWhiteSpace($v) -or $v -eq 'system') { exit 1 }`),
			Commands: []CommandSpec{
				powerShell(`$ErrorActionPreference='Stop'
function Invoke-Native {
    param([Parameter(Mandatory=$true)][string]$File, [Parameter(Mandatory=$true)][string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}
$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
$out = Invoke-Native 'pyenv' @('install', '-l')
$v = $out -split '\r?\n' | Where-Object { $_ -match '^\s*3\.\d+\.\d+\s*$' } | Sort-Object -Descending | Select-Object -First 1
if (-not $v) { throw 'No stable Python 3.x version found' }
$v = $v.Trim()
Invoke-Native 'pyenv' @('install', '-s', $v)
Invoke-Native 'pyenv' @('global', $v)
Invoke-Native 'pyenv' @('exec', 'python', '-m', 'pip', 'install', '--upgrade', 'pip')
`),
			},
		},
		{
			Name:  "semgrep",
			Phase: PhaseHost,
			Probe: powerShell(`$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User'); pyenv exec semgrep --version | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User'); pyenv exec pip install semgrep`),
			},
		},
	}

	actions = append(actions, Action{
		Name:  "VLC default media player associations",
		Phase: PhaseHost,
		Probe: powerShell(`$stateDir = Join-Path $env:ProgramData 'bootstrap_windows_env'; if (-not (Test-Path (Join-Path $stateDir 'vlc-default-media.done'))) { exit 1 }`),
		Commands: []CommandSpec{
			powerShell(`$stateDir = Join-Path $env:ProgramData 'bootstrap_windows_env'
if (-not (Test-Path $stateDir)) { New-Item -ItemType Directory -Force -Path $stateDir | Out-Null }
$vlcPaths = @(
    (Join-Path $env:ProgramFiles 'VideoLAN\VLC\vlc.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'VideoLAN\VLC\vlc.exe')
) | Where-Object { $_ -and (Test-Path $_) }
if ($vlcPaths.Count -eq 0 -and -not (Get-Command vlc.exe -ErrorAction SilentlyContinue)) {
    throw 'VLC is not installed or is not discoverable.'
}
$extensions = @(
    '.3g2', '.3gp', '.3gp2', '.3gpp', '.amv', '.asf', '.avi', '.divx', '.flac', '.flv', '.m2ts', '.m4a', '.m4v', '.mka', '.mkv',
    '.mov', '.mp2', '.mp3', '.mp4', '.mp4v', '.mpeg', '.mpg', '.mts', '.oga',
    '.ogg', '.ogm', '.ogv', '.opus', '.ts', '.vob', '.wav', '.webm', '.wma', '.wmv'
)
$registered = @{}
$capabilityPaths = @(
    'HKLM:\SOFTWARE\Clients\Media\VLC\Capabilities\FileAssociations',
    'HKLM:\SOFTWARE\VideoLAN\VLC\Capabilities\FileAssociations',
    'HKLM:\SOFTWARE\Applications\vlc.exe\Capabilities\FileAssociations',
    'HKCU:\SOFTWARE\Clients\Media\VLC\Capabilities\FileAssociations',
    'HKCU:\SOFTWARE\Applications\vlc.exe\Capabilities\FileAssociations'
)
foreach ($path in $capabilityPaths) {
    if (-not (Test-Path $path)) { continue }
    $props = Get-ItemProperty $path
    foreach ($ext in $extensions) {
        if ($props.PSObject.Properties.Name -contains $ext) {
            $registered[$ext] = [string]$props.$ext
        }
    }
}
$xmlPath = Join-Path $stateDir 'VlcDefaultAssociations.xml'
$lines = New-Object System.Collections.Generic.List[string]
$lines.Add('<?xml version="1.0" encoding="UTF-8"?>')
$lines.Add('<DefaultAssociations>')
foreach ($ext in $extensions) {
    $progId = $registered[$ext]
    if ([string]::IsNullOrWhiteSpace($progId)) {
        $progId = 'VLC' + $ext
    }
    $safeExt = [System.Security.SecurityElement]::Escape($ext)
    $safeProgId = [System.Security.SecurityElement]::Escape($progId)
    $lines.Add("  <Association Identifier=""$safeExt"" ProgId=""$safeProgId"" ApplicationName=""VLC media player"" />")
}
$lines.Add('</DefaultAssociations>')
$lines | Set-Content -Path $xmlPath -Encoding UTF8
dism.exe /Online /Import-DefaultAppAssociations:$xmlPath
if ($LASTEXITCODE -ne 0) {
    throw "dism.exe default association import failed with exit code $LASTEXITCODE"
}
Set-Content -Path (Join-Path $stateDir 'vlc-default-media.done') -Value (Get-Date -Format o) -Encoding UTF8
Write-Output "Imported VLC default media associations from $xmlPath. Windows may still require user confirmation for an existing signed-in profile."`),
		},
	})

	return actions
}

func CustomActions(opts Options) []Action {
	actions := []Action{
		{
			Name:  "agy",
			Phase: PhaseCustom,
			AI:    true,
			Probe: powerShell(`agy --version`),
			Commands: []CommandSpec{
				powerShell(`Invoke-RestMethod https://antigravity.google/cli/install.ps1 | Invoke-Expression`),
			},
		},
		{
			Name:  "Playwright CLI",
			Phase: PhaseCustom,
			Probe: powerShell(`npm list --global playwright --depth=0 | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`npm install --global playwright`),
				checkedNativePowerShell(`playwright install`),
			},
		},
		{
			Name:  "GitHub CLI gh-repo-bootstrap extension",
			Phase: PhaseCustom,
			Probe: powerShell(`if (-not (gh extension list | Select-String 'JMR-dev/gh-repo-bootstrap')) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`gh extension install JMR-dev/gh-repo-bootstrap`),
			},
		},
		{
			Name:  "Claude Code",
			Phase: PhaseCustom,
			AI:    true,
			Probe: powerShell(`npm list --global '@anthropic-ai/claude-code' --depth=0 | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`npm install --global '@anthropic-ai/claude-code'`),
			},
		},
		{
			Name:  "OpenAI Codex CLI",
			Phase: PhaseCustom,
			AI:    true,
			Probe: powerShell(`npm list --global '@openai/codex' --depth=0 | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`npm install --global '@openai/codex'`),
			},
		},
		{
			Name:  "GitHub Copilot CLI",
			Phase: PhaseCustom,
			AI:    true,
			Probe: powerShell(`npm list --global '@github/copilot' --depth=0 | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`npm install --global '@github/copilot'`),
			},
		},
	}
	if !opts.NoAI {
		return actions
	}
	filtered := make([]Action, 0, len(actions))
	for _, action := range actions {
		if !action.AI {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

type ActionState struct {
	Action    Action
	Installed bool
	CheckErr  error
}

func CheckActions(ctx context.Context, runner Runner, actions []Action) []ActionState {
	states := make([]ActionState, 0, len(actions))
	for _, action := range actions {
		result := runner.Run(ctx, action.Probe.Name, action.Probe.Args...)
		states = append(states, ActionState{Action: action, Installed: result.Err == nil})
	}
	return states
}

func PendingActions(states []ActionState) []Action {
	var pending []Action
	for _, state := range states {
		if !state.Installed {
			pending = append(pending, state.Action)
		}
	}
	return pending
}

func UnknownActionStates(actions []Action) []ActionState {
	states := make([]ActionState, 0, len(actions))
	for _, action := range actions {
		states = append(states, ActionState{Action: action})
	}
	return states
}

func ExecuteActions(ctx context.Context, runner Runner, actions []Action) Issues {
	var issues Issues
	for _, action := range actions {
		for _, command := range action.Commands {
			result := runner.Run(ctx, command.Name, command.Args...)
			if result.Err != nil {
				output := strings.TrimSpace(result.CombinedOutput())
				issues = append(issues, Issue{Step: action.Name, Err: fmt.Errorf("%w: %s", result.Err, output)})
				break
			}
		}
	}
	return issues
}
