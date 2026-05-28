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

const refreshPath = `$env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User'); `

func powerShell(script string) CommandSpec {
	return CommandSpec{
		Name: "powershell.exe",
		Args: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", refreshPath + script},
	}
}

func checkedNativePowerShell(command string) CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'; ` + command + `; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }`)
}

func HostActions() []Action {
	return []Action{
		{
			Name:  "Node.js LTS through fnm",
			Phase: PhaseHost,
			Probe: checkedNativePowerShell(`fnm exec --using=lts-latest node --version`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`fnm install --lts`),
				checkedNativePowerShell(`fnm default lts-latest`),
			},
		},
		{
			Name:  "pyenv-win through Scoop or Chocolatey",
			Phase: PhaseHost,
			Probe: checkedNativePowerShell(`pyenv --version`),
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
    Invoke-Native 'winget' @('install', '--id', 'Chocolatey.Chocolatey', '--exact', '--source', 'winget', '--accept-source-agreements', '--accept-package-agreements', '--disable-interactivity')
    $env:Path=[Environment]::GetEnvironmentVariable('Path','Machine')+';'+[Environment]::GetEnvironmentVariable('Path','User')
    Invoke-Native 'choco' @('install', 'pyenv-win', '-y', '--no-progress')
}`),
			},
		},
		{
			Name:  "latest stable Python through pyenv-win",
			Phase: PhaseHost,
			Probe: powerShell(`$v = pyenv version-name 2>$null; if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($v) -or $v.Trim() -eq 'system') { exit 1 }`),
			Commands: []CommandSpec{
				powerShell(`$ErrorActionPreference='Stop'
function Invoke-Native {
    param([Parameter(Mandatory=$true)][string]$File, [Parameter(Mandatory=$true)][string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}
$v=(pyenv install --list | Select-String '^\s*3\.\d+\.\d+\s*$' | ForEach-Object {$_.Line.Trim()} | Sort-Object {[version]$_} -Descending | Select-Object -First 1)
if (-not $v) { throw 'no stable Python version found' }
Invoke-Native 'pyenv' @('install', '-s', $v)
Invoke-Native 'pyenv' @('global', $v)`),
			},
		},
		{
			Name:  "VLC default media player associations",
			Phase: PhaseHost,
			Probe: powerShell(`$marker = Join-Path $env:ProgramData 'bootstrap_windows_env\vlc-default-media.done'; if (-not (Test-Path $marker)) { exit 1 }`),
			Commands: []CommandSpec{
				powerShell(`$ErrorActionPreference='Stop'
$stateDir = Join-Path $env:ProgramData 'bootstrap_windows_env'
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
$vlcPaths = @(
    (Join-Path $env:ProgramFiles 'VideoLAN\VLC\vlc.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'VideoLAN\VLC\vlc.exe')
) | Where-Object { $_ -and (Test-Path $_) }
if ($vlcPaths.Count -eq 0 -and -not (Get-Command vlc.exe -ErrorAction SilentlyContinue)) {
    throw 'VLC is not installed or is not discoverable.'
}
$extensions = @(
    '.3g2', '.3gp', '.aac', '.aiff', '.alac', '.amr', '.ape', '.asf', '.au',
    '.avi', '.divx', '.flac', '.flv', '.m2ts', '.m4a', '.m4v', '.mka', '.mkv',
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
		},
	}
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
				checkedNativePowerShell(`fnm exec --using=lts-latest npm install --global playwright`),
				checkedNativePowerShell(`fnm exec --using=lts-latest playwright install`),
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
				checkedNativePowerShell(`fnm exec --using=lts-latest npm install --global '@anthropic-ai/claude-code'`),
			},
		},
		{
			Name:  "OpenAI Codex CLI",
			Phase: PhaseCustom,
			AI:    true,
			Probe: powerShell(`npm list --global '@openai/codex' --depth=0 | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`fnm exec --using=lts-latest npm install --global '@openai/codex'`),
			},
		},
		{
			Name:  "GitHub Copilot CLI",
			Phase: PhaseCustom,
			AI:    true,
			Probe: powerShell(`npm list --global '@github/copilot' --depth=0 | Out-Null; if ($LASTEXITCODE -ne 0) { exit 1 }`),
			Commands: []CommandSpec{
				checkedNativePowerShell(`fnm exec --using=lts-latest npm install --global '@github/copilot'`),
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
