package bootstrap

import (
	"context"
	"fmt"
	"strings"
)

type OSPhaseResult struct {
	Notices []string
	Issues  Issues
	Stop    bool
}

func RunOSPhase(ctx context.Context, runner Runner) OSPhaseResult {
	var result OSPhaseResult
	if issue := runOSCommand(ctx, runner, "install Windows updates", WindowsUpdateCommand()); issue.Err != nil {
		result.Issues = append(result.Issues, issue)
	}
	reboot, err := DetectWindowsRebootRequired(ctx, runner)
	if err != nil {
		result.Issues = append(result.Issues, Issue{Step: "detect pending reboot", Err: err})
	} else if reboot {
		result.Notices = append(result.Notices, "Windows Update installed changes that require a reboot. Restart Windows, then rerun this bootstrapper before package installation.")
		result.Stop = true
		return result
	}
	if issue := runOSCommand(ctx, runner, "create restore point before OS configuration", RestorePointCommand("bootstrap_windows_env: before OS configuration")); issue.Err != nil {
		result.Issues = append(result.Issues, issue)
		result.Stop = true
		return result
	}
	if issue := runOSCommand(ctx, runner, "run Chris Titus Tech WinUtil", WinUtilCommand()); issue.Err != nil {
		result.Issues = append(result.Issues, issue)
	}
	for _, step := range []struct {
		name  string
		cmd   CommandSpec
		fatal bool
	}{
		{"apply power settings", PowerPolicyCommand(), false},
		{"enable Developer Mode and Windows sudo", DeveloperModeCommand(), false},
		{"apply Windows Update policy", WindowsUpdatePolicyCommand(), false},
		{"apply Explorer and theme settings", ExplorerThemeCommand(), false},
		{"set Vivaldi default browser associations", VivaldiDefaultBrowserCommand(), false},
		{"update Microsoft Store apps", StoreUpdateCommand(), false},
		{"apply taskbar pins", TaskbarCommand(), false},
		{"create restore point before package installation", RestorePointCommand("bootstrap_windows_env: before package installation"), true},
	} {
		if issue := runOSCommand(ctx, runner, step.name, step.cmd); issue.Err != nil {
			result.Issues = append(result.Issues, issue)
			if step.fatal {
				result.Stop = true
				return result
			}
		}
	}
	return result
}

func runOSCommand(ctx context.Context, runner Runner, step string, cmd CommandSpec) Issue {
	res := runner.Run(ctx, cmd.Name, cmd.Args...)
	if res.Err != nil {
		return Issue{Step: step, Err: fmt.Errorf("%w: %s", res.Err, res.CombinedOutput())}
	}
	return Issue{}
}

func WindowsUpdateCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
$serviceManager = New-Object -ComObject Microsoft.Update.ServiceManager
try {
    $serviceManager.ClientApplicationID = 'bootstrap_windows_env'
    $serviceManager.AddService2('7971f918-a847-4430-9279-4a52d1efe18d', 7, '') | Out-Null
    Write-Output 'Microsoft Update enabled for other Microsoft products.'
} catch {
    Write-Output "Microsoft Update service registration skipped: $($_.Exception.Message)"
}
$session = New-Object -ComObject Microsoft.Update.Session
$session.ClientApplicationID = 'bootstrap_windows_env'
$searcher = $session.CreateUpdateSearcher()
$search = $searcher.Search("IsInstalled=0 and IsHidden=0")
$updates = New-Object -ComObject Microsoft.Update.UpdateColl
for ($i = 0; $i -lt $search.Updates.Count; $i++) {
    $update = $search.Updates.Item($i)
    if ($update.InstallationBehavior.CanRequestUserInput) {
        Write-Output "Skipping interactive update: $($update.Title)"
        continue
    }
    if (-not $update.EulaAccepted) {
        $update.AcceptEula()
    }
    [void]$updates.Add($update)
}
if ($updates.Count -eq 0) {
    Write-Output 'No applicable Windows updates found.'
    exit 0
}
Write-Output "Downloading $($updates.Count) Windows update(s)."
$downloader = $session.CreateUpdateDownloader()
$downloader.Updates = $updates
$downloadResult = $downloader.Download()
Write-Output "Windows Update download result code: $($downloadResult.ResultCode)."
if ($downloadResult.ResultCode -ne 2) {
    throw "Windows Update download did not fully succeed; result code: $($downloadResult.ResultCode)"
}
Write-Output "Installing $($updates.Count) Windows update(s)."
$installer = $session.CreateUpdateInstaller()
$installer.Updates = $updates
$installer.ForceQuiet = $true
$installResult = $installer.Install()
Write-Output "Windows Update result code: $($installResult.ResultCode); reboot required: $($installResult.RebootRequired)."
if ($installResult.ResultCode -ne 2) {
    throw "Windows Update install did not fully succeed; result code: $($installResult.ResultCode)"
}
exit 0`)
}

func DetectWindowsRebootRequired(ctx context.Context, runner Runner) (bool, error) {
	cmd := powerShell(`$paths = @(
  'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending',
  'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'
)
$pending = $false
foreach ($path in $paths) {
    if (Test-Path $path) { $pending = $true }
}
$sessionManager = Get-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager' -Name PendingFileRenameOperations -ErrorAction SilentlyContinue
if ($sessionManager.PendingFileRenameOperations) { $pending = $true }
if ($pending) { 'true' } else { 'false' }`)
	res := runner.Run(ctx, cmd.Name, cmd.Args...)
	if res.Err != nil {
		return false, fmt.Errorf("%w: %s", res.Err, res.CombinedOutput())
	}
	return containsLine(res.Stdout, "true"), nil
}

func RestorePointCommand(description string) CommandSpec {
	return powerShell(fmt.Sprintf(`$ErrorActionPreference='Stop'
try { Enable-ComputerRestore -Drive "$env:SystemDrive\" -ErrorAction SilentlyContinue } catch {}
$restorePolicy = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore'
New-Item -Path $restorePolicy -Force | Out-Null
Set-ItemProperty -Path $restorePolicy -Name SystemRestorePointCreationFrequency -Type DWord -Value 0
Checkpoint-Computer -Description %q -RestorePointType 'MODIFY_SETTINGS'
Write-Output 'Restore point created: %s'`, description, description))
}

func WinUtilCommand() CommandSpec {
	config := mustInstallAssetPath("assets/winutil-sane-default.json")
	return powerShell(`$ErrorActionPreference='Stop'
$config = '` + psSingleQuote(config) + `'
& ([ScriptBlock]::Create((irm 'https://christitus.com/win'))) -Config $config -Run`)
}

func PowerPolicyCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
function Invoke-Native {
    param([Parameter(Mandatory=$true)][string]$File, [Parameter(Mandatory=$true)][string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}
$stateDir = Join-Path $env:ProgramData 'bootstrap_windows_env'
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
$policyScript = Join-Path $stateDir 'Apply-PowerPolicy.ps1'
@'
$ErrorActionPreference='Stop'
function Invoke-Native {
    param([Parameter(Mandatory=$true)][string]$File, [Parameter(Mandatory=$true)][string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}
$isLaptop = @(Get-CimInstance -ClassName Win32_Battery -ErrorAction SilentlyContinue).Count -gt 0
$onBattery = $false
if ($isLaptop) {
    $batteries = @(Get-CimInstance -ClassName Win32_Battery -ErrorAction SilentlyContinue)
    $onBattery = ($batteries | Where-Object { $_.BatteryStatus -in 1, 4, 5 }).Count -gt 0
}
if ($isLaptop -and $onBattery) {
    Invoke-Native 'powercfg.exe' @('/setactive', 'SCHEME_MAX')
    $lockSeconds = 120
} else {
    Invoke-Native 'powercfg.exe' @('/setactive', 'SCHEME_MIN')
    $lockSeconds = 300
}
New-Item -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Force | Out-Null
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Name InactivityTimeoutSecs -Type DWord -Value $lockSeconds
'@ | Set-Content -Path $policyScript -Encoding UTF8
Invoke-Native 'powercfg.exe' @('/hibernate', 'on')
New-Item -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\FlyoutMenuSettings' -Force | Out-Null
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\FlyoutMenuSettings' -Name ShowHibernateOption -Type DWord -Value 1
foreach ($scheme in @('SCHEME_MIN', 'SCHEME_MAX')) {
    Invoke-Native 'powercfg.exe' @('/setacvalueindex', $scheme, 'SUB_VIDEO', 'VIDEOIDLE', '900')
    Invoke-Native 'powercfg.exe' @('/setdcvalueindex', $scheme, 'SUB_VIDEO', 'VIDEOIDLE', '300')
    Invoke-Native 'powercfg.exe' @('/setacvalueindex', $scheme, 'SUB_SLEEP', 'STANDBYIDLE', '0')
    Invoke-Native 'powercfg.exe' @('/setdcvalueindex', $scheme, 'SUB_SLEEP', 'STANDBYIDLE', '900')
    Invoke-Native 'powercfg.exe' @('/setacvalueindex', $scheme, 'SUB_BUTTONS', 'LIDACTION', '0')
    Invoke-Native 'powercfg.exe' @('/setdcvalueindex', $scheme, 'SUB_BUTTONS', 'LIDACTION', '1')
    Invoke-Native 'powercfg.exe' @('/setacvalueindex', $scheme, 'SUB_BUTTONS', 'PBUTTONACTION', '3')
    Invoke-Native 'powercfg.exe' @('/setdcvalueindex', $scheme, 'SUB_BUTTONS', 'PBUTTONACTION', '3')
    Invoke-Native 'powercfg.exe' @('/setacvalueindex', $scheme, 'SUB_BUTTONS', 'SBUTTONACTION', '1')
    Invoke-Native 'powercfg.exe' @('/setdcvalueindex', $scheme, 'SUB_BUTTONS', 'SBUTTONACTION', '1')
}
Invoke-Native 'powershell.exe' @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $policyScript)
$taskArg = '-NoLogo -NoProfile -ExecutionPolicy Bypass -File "' + $policyScript + '"'
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $taskArg
$logonTrigger = New-ScheduledTaskTrigger -AtLogOn
$repeatTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 5) -RepetitionDuration (New-TimeSpan -Days 3650)
Register-ScheduledTask -TaskName 'BootstrapWindowsEnv Power Policy Refresh' -Action $action -Trigger @($logonTrigger, $repeatTrigger) -RunLevel Highest -Force | Out-Null`)
}

func WindowsUpdatePolicyCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
$wu = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate'
$au = Join-Path $wu 'AU'
New-Item -Path $wu -Force | Out-Null
New-Item -Path $au -Force | Out-Null
Set-ItemProperty -Path $wu -Name DeferFeatureUpdates -Type DWord -Value 1
Set-ItemProperty -Path $wu -Name DeferFeatureUpdatesPeriodInDays -Type DWord -Value 90
Set-ItemProperty -Path $wu -Name BranchReadinessLevel -Type DWord -Value 16
Set-ItemProperty -Path $wu -Name SetActiveHours -Type DWord -Value 1
Set-ItemProperty -Path $wu -Name ActiveHoursStart -Type DWord -Value 6
Set-ItemProperty -Path $wu -Name ActiveHoursEnd -Type DWord -Value 22
Set-ItemProperty -Path $au -Name AllowMUUpdateService -Type DWord -Value 1
$ux = 'HKLM:\SOFTWARE\Microsoft\WindowsUpdate\UX\Settings'
New-Item -Path $ux -Force | Out-Null
Set-ItemProperty -Path $ux -Name ActiveHoursStart -Type DWord -Value 6
Set-ItemProperty -Path $ux -Name ActiveHoursEnd -Type DWord -Value 22`)
}

func DeveloperModeCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
$unlock = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock'
New-Item -Path $unlock -Force | Out-Null
Set-ItemProperty -Path $unlock -Name AllowDevelopmentWithoutDevLicense -Type DWord -Value 1
Set-ItemProperty -Path $unlock -Name AllowAllTrustedApps -Type DWord -Value 1
$devSettings = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\DeveloperSettings'
New-Item -Path $devSettings -Force | Out-Null
Set-ItemProperty -Path $devSettings -Name EnableSudo -Type DWord -Value 1
Set-ItemProperty -Path $devSettings -Name SudoMode -Type DWord -Value 1
if (Get-Command sudo.exe -ErrorAction SilentlyContinue) {
    sudo.exe config --enable normal
    if ($LASTEXITCODE -ne 0) {
        throw "sudo.exe config failed with exit code $LASTEXITCODE"
    }
} else {
    Write-Output 'sudo.exe is not present on this Windows build; registry preference was written for builds that support Windows sudo.'
}`)
}

func ExplorerThemeCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
$classic = 'HKCU:\Software\Classes\CLSID\{86ca1aa0-34aa-4e8b-a509-50c905bae2a2}\InprocServer32'
New-Item -Path $classic -Force | Out-Null
Set-Item -Path $classic -Value ''
$theme = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize'
New-Item -Path $theme -Force | Out-Null
Set-ItemProperty -Path $theme -Name AppsUseLightTheme -Type DWord -Value 0
Set-ItemProperty -Path $theme -Name SystemUsesLightTheme -Type DWord -Value 0
Stop-Process -Name explorer -Force -ErrorAction SilentlyContinue`)
}

func VivaldiDefaultBrowserCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
$progId = 'VivaldiHTM'
$htmlProgId = $progId
$roots = @('HKLM:\SOFTWARE\Clients\StartMenuInternet', 'HKCU:\SOFTWARE\Clients\StartMenuInternet')
foreach ($root in $roots) {
    if (-not (Test-Path $root)) { continue }
    Get-ChildItem $root -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -like '*Vivaldi*' } | ForEach-Object {
        $cap = Join-Path $_.PSPath 'Capabilities'
        $url = Join-Path $cap 'URLAssociations'
        $file = Join-Path $cap 'FileAssociations'
        if (Test-Path $url) {
            $props = Get-ItemProperty $url
            if ($props.http) { $script:progId = $props.http }
        }
        if (Test-Path $file) {
            $props = Get-ItemProperty $file
            if ($props.'.html') { $script:htmlProgId = $props.'.html' }
        }
    }
}
$xml = Join-Path $env:ProgramData 'bootstrap_windows_env\DefaultAssociations.xml'
New-Item -ItemType Directory -Force -Path (Split-Path $xml) | Out-Null
@"
<?xml version="1.0" encoding="UTF-8"?>
<DefaultAssociations>
  <Association Identifier=".htm" ProgId="$htmlProgId" ApplicationName="Vivaldi" />
  <Association Identifier=".html" ProgId="$htmlProgId" ApplicationName="Vivaldi" />
  <Association Identifier="http" ProgId="$progId" ApplicationName="Vivaldi" />
  <Association Identifier="https" ProgId="$progId" ApplicationName="Vivaldi" />
</DefaultAssociations>
"@ | Set-Content -Path $xml -Encoding UTF8
dism.exe /Online /Import-DefaultAppAssociations:$xml
if ($LASTEXITCODE -ne 0) {
    throw "dism.exe default association import failed with exit code $LASTEXITCODE"
}
Write-Output 'Imported Vivaldi default browser associations. Windows may still require user confirmation for an existing signed-in profile.'`)
}

func StoreUpdateCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
winget source update
if ($LASTEXITCODE -ne 0) {
    throw "winget source update failed with exit code $LASTEXITCODE"
}
winget upgrade --all --source msstore --accept-source-agreements --accept-package-agreements --disable-interactivity
if ($LASTEXITCODE -ne 0) {
    throw "winget Store app upgrade failed with exit code $LASTEXITCODE"
}
try {
    $mgr = Get-CimInstance -Namespace 'Root\cimv2\mdm\dmmap' -ClassName 'MDM_EnterpriseModernAppManagement_AppManagement01' -ErrorAction Stop
    Invoke-CimMethod -InputObject $mgr -MethodName UpdateScanMethod | Out-Null
} catch {
    Write-Output "Store update scan fallback skipped: $($_.Exception.Message)"
}
try { Start-Process wsreset.exe -ArgumentList '-i' -WindowStyle Hidden -Wait } catch { Write-Output "wsreset fallback skipped: $($_.Exception.Message)" }`)
}

func TaskbarCommand() CommandSpec {
	return powerShell(`$ErrorActionPreference='Stop'
function Invoke-TaskbarVerb {
    param([string]$Path, [string]$Pattern)
    if (-not (Test-Path $Path)) { return $false }
    $shell = New-Object -ComObject Shell.Application
    $folder = $shell.Namespace((Split-Path $Path))
    $item = $folder.ParseName((Split-Path $Path -Leaf))
    if (-not $item) { return $false }
    $verb = $item.Verbs() | Where-Object { ($_.Name -replace '&','') -match $Pattern } | Select-Object -First 1
    if ($verb) { $verb.DoIt(); return $true }
    return $false
}
$failures = New-Object System.Collections.Generic.List[string]
$edgePaths = @(
    "$env:ProgramFiles (x86)\Microsoft\Edge\Application\msedge.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
)
foreach ($edge in $edgePaths) {
    if ((Test-Path $edge) -and -not (Invoke-TaskbarVerb -Path $edge -Pattern 'Unpin from taskbar|Unpin from Taskbar')) {
        $failures.Add("could not unpin Edge from $edge")
    }
}
if (-not (Invoke-TaskbarVerb -Path "$env:windir\System32\Taskmgr.exe" -Pattern 'Pin to taskbar|Pin to Taskbar')) {
    $failures.Add('could not pin Task Manager to taskbar')
}
$settingsShortcut = Join-Path $env:ProgramData 'bootstrap_windows_env\Settings.lnk'
New-Item -ItemType Directory -Force -Path (Split-Path $settingsShortcut) | Out-Null
$wsh = New-Object -ComObject WScript.Shell
$shortcut = $wsh.CreateShortcut($settingsShortcut)
$shortcut.TargetPath = 'explorer.exe'
$shortcut.Arguments = 'ms-settings:'
$shortcut.IconLocation = "$env:windir\ImmersiveControlPanel\SystemSettings.exe"
$shortcut.Save()
if (-not (Invoke-TaskbarVerb -Path $settingsShortcut -Pattern 'Pin to taskbar|Pin to Taskbar')) {
    $failures.Add('could not pin Settings to taskbar')
}
if ($failures.Count -gt 0) {
    throw ($failures -join '; ')
}`)
}

func mustInstallAssetPath(asset string) string {
	path, err := writeInstallAsset(asset)
	if err != nil {
		return asset
	}
	return path
}

func psSingleQuote(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func containsLine(output, needle string) bool {
	for _, line := range stringsSplitLines(output) {
		if line == needle {
			return true
		}
	}
	return false
}

func stringsSplitLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		lines = append(lines, strings.TrimSpace(strings.ToLower(line)))
	}
	return lines
}
