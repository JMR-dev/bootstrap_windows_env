<#
.SYNOPSIS
    Installs HashiCorp Packer on Windows by downloading the release zip,
    extracting it to Program Files, and adding that directory to the system
    PATH.

.DESCRIPTION
    Winget does not publish HashiCorp Packer, so this helper does the install
    by hand. It is idempotent: if the requested version is already installed
    at the target location it just re-asserts the PATH entry and exits.

    Requires an elevated PowerShell session (writes under Program Files and
    modifies the machine-scoped PATH environment variable).

.PARAMETER Version
    Packer release to install (e.g. "1.15.3"). The download URL is computed
    as https://releases.hashicorp.com/packer/<Version>/packer_<Version>_windows_amd64.zip.

.PARAMETER InstallRoot
    Directory under which packer.exe is placed. Defaults to
    "$env:ProgramFiles\Packer". This directory is what gets added to the
    system PATH.

.PARAMETER SkipOscdimg
    Skip installing the Windows ADK Deployment Tools (which provide
    oscdimg.exe). Packer's hyperv-iso builder needs an ISO-creation tool
    (xorriso, mkisofs, hdiutil, or oscdimg) on PATH when using cd_files,
    so by default this script also installs oscdimg via the Windows ADK
    bootstrapper with only the DeploymentTools feature selected (~70 MB
    on disk).

.PARAMETER AdkSetupUrl
    Override the URL used to download the Windows ADK setup bootstrapper.
    Defaults to Microsoft's stable fwlink for the Windows 11 ADK.

.EXAMPLE
    # Install the default version into "C:\Program Files\Packer".
    .\install-packer.ps1

.EXAMPLE
    # Install a specific version.
    .\install-packer.ps1 -Version 1.14.0

.EXAMPLE
    # Install Packer only; skip the Windows ADK Deployment Tools install.
    .\install-packer.ps1 -SkipOscdimg
#>
[CmdletBinding()]
param(
    [string] $Version     = '1.15.3',
    [string] $InstallRoot = (Join-Path $env:ProgramFiles 'Packer'),
    [switch] $SkipOscdimg,
    [string] $AdkSetupUrl = 'https://go.microsoft.com/fwlink/?linkid=2289980'
)

$ErrorActionPreference = 'Stop'

function Assert-Elevated {
    $identity  = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "install-packer.ps1 must be run from an elevated PowerShell session (writes to Program Files and the machine PATH)."
    }
}

function Get-InstalledPackerVersion {
    param([string] $Root)
    $exe = Join-Path $Root 'packer.exe'
    if (-not (Test-Path -LiteralPath $exe)) { return $null }
    try {
        $output = & $exe version 2>$null
    } catch {
        return $null
    }
    foreach ($line in $output) {
        if ($line -match 'Packer\s+v?([0-9]+\.[0-9]+\.[0-9]+)') {
            return $Matches[1]
        }
    }
    return $null
}

function Add-ToMachinePath {
    param([string] $Directory)
    $current = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    $parts   = @()
    if ($current) {
        $parts = $current.Split(';') | Where-Object { $_ -ne '' }
    }
    if ($parts -contains $Directory) {
        Write-Host "system PATH already contains $Directory"
        return
    }
    $newPath = ($parts + $Directory) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'Machine')
    # Make the change visible in the current session as well so the caller
    # can immediately run `packer` without opening a new shell.
    if (-not (($env:Path -split ';') -contains $Directory)) {
        $env:Path = "$env:Path;$Directory"
    }
    Write-Host "added $Directory to the system PATH (machine scope)."
}

function Get-OscdimgPath {
    $candidates = @(
        (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\Assessment and Deployment Kit\Deployment Tools\amd64\Oscdimg\oscdimg.exe'),
        (Join-Path ${env:ProgramFiles}      'Windows Kits\10\Assessment and Deployment Kit\Deployment Tools\amd64\Oscdimg\oscdimg.exe')
    )
    foreach ($c in $candidates) {
        if ($c -and (Test-Path -LiteralPath $c)) { return $c }
    }
    return $null
}

function Install-Oscdimg {
    param([string] $SetupUrl)

    $existing = Get-OscdimgPath
    if ($existing) {
        Write-Host "oscdimg.exe already present at $existing"
        Add-ToMachinePath -Directory (Split-Path -Parent $existing)
        return
    }

    $tmp     = Join-Path ([IO.Path]::GetTempPath()) ("install-adk-" + [Guid]::NewGuid())
    $setup   = Join-Path $tmp 'adksetup.exe'
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null
    try {
        Write-Host "downloading Windows ADK bootstrapper from $SetupUrl"
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -UseBasicParsing -Uri $SetupUrl -OutFile $setup

        Write-Host "installing Windows ADK Deployment Tools (this may take several minutes)"
        # /features OptionId.DeploymentTools => oscdimg only (~70 MB)
        # /quiet /norestart /ceip off        => unattended, no telemetry prompt
        $args = @('/features','OptionId.DeploymentTools','/quiet','/norestart','/ceip','off')
        $proc = Start-Process -FilePath $setup -ArgumentList $args -Wait -PassThru
        if ($proc.ExitCode -ne 0) {
            throw "Windows ADK setup exited with code $($proc.ExitCode)."
        }
    }
    finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    $installed = Get-OscdimgPath
    if (-not $installed) {
        throw "Windows ADK setup completed but oscdimg.exe was not found in the expected location."
    }
    Write-Host "installed oscdimg.exe at $installed"
    Add-ToMachinePath -Directory (Split-Path -Parent $installed)
}

Assert-Elevated

# Idempotency: if the target version is already installed, just make sure the
# PATH entry is in place and return.
$existing = Get-InstalledPackerVersion -Root $InstallRoot
if ($existing -eq $Version) {
    Write-Host "packer $Version already installed at $InstallRoot."
    Add-ToMachinePath -Directory $InstallRoot
    if (-not $SkipOscdimg) { Install-Oscdimg -SetupUrl $AdkSetupUrl }
    return
}
if ($existing) {
    Write-Host "replacing packer $existing at $InstallRoot with $Version."
}

$url     = "https://releases.hashicorp.com/packer/$Version/packer_${Version}_windows_amd64.zip"
$tmpRoot = Join-Path ([IO.Path]::GetTempPath()) ("install-packer-" + [Guid]::NewGuid())
$zipPath = Join-Path $tmpRoot "packer_${Version}.zip"

New-Item -ItemType Directory -Path $tmpRoot -Force | Out-Null
try {
    Write-Host "downloading $url"
    # TLS 1.2 is required to talk to releases.hashicorp.com on stock
    # Windows 10/Server 2019 PowerShell sessions.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $zipPath

    Write-Host "extracting to $tmpRoot"
    Expand-Archive -LiteralPath $zipPath -DestinationPath $tmpRoot -Force

    $srcExe = Join-Path $tmpRoot 'packer.exe'
    if (-not (Test-Path -LiteralPath $srcExe)) {
        throw "packer.exe not found inside $zipPath after extraction."
    }

    New-Item -ItemType Directory -Path $InstallRoot -Force | Out-Null
    Copy-Item -LiteralPath $srcExe -Destination (Join-Path $InstallRoot 'packer.exe') -Force
    Write-Host "installed packer.exe to $InstallRoot"

    Add-ToMachinePath -Directory $InstallRoot

    $installed = Get-InstalledPackerVersion -Root $InstallRoot
    if ($installed -ne $Version) {
        Write-Warning "post-install version check returned '$installed' (expected $Version)."
    } else {
        Write-Host "verified: packer $installed available at $(Join-Path $InstallRoot 'packer.exe')."
    }
}
finally {
    Remove-Item -LiteralPath $tmpRoot -Recurse -Force -ErrorAction SilentlyContinue
}

if (-not $SkipOscdimg) { Install-Oscdimg -SetupUrl $AdkSetupUrl }
