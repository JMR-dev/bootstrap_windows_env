# Generic OS tweaks applied after WinRM is reachable.
#
# Idempotent; safe to rerun.

$ErrorActionPreference = 'Stop'

# Make sure the remote-elevated token policy survives any unattended changes.
$policy = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System'
New-ItemProperty -Path $policy -Name 'LocalAccountTokenFilterPolicy' `
  -PropertyType DWord -Value 1 -Force | Out-Null

# Disable telemetry "Connected User Experiences" service to avoid background
# network noise that competes with the bootstrapper.
$svc = Get-Service -Name DiagTrack -ErrorAction SilentlyContinue
if ($svc) {
    Set-Service -Name DiagTrack -StartupType Disabled
    Stop-Service -Name DiagTrack -Force -ErrorAction SilentlyContinue
}

# Disable SmartScreen network checks (slows winget installs in the lab).
$ss = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\System'
New-Item -Path $ss -Force | Out-Null
New-ItemProperty -Path $ss -Name 'EnableSmartScreen' -PropertyType DWord -Value 0 -Force | Out-Null

# Force NTP resync; Hyper-V time-sync after sysprep can drift, which breaks
# TLS handshakes against winget / GitHub.
Start-Service -Name w32time -ErrorAction SilentlyContinue
& w32tm /resync /force 2>&1 | Out-Null
