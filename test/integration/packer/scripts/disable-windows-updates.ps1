# Disable Windows Update on the gold image so it doesn't fight with the
# bootstrapper's own WindowsUpdateCommand step at integration-test time.
#
# This is the precise mitigation for the 6-hour CI hang investigation: by
# the time the bootstrapper runs, no other process is holding the WU agent.

$ErrorActionPreference = 'Stop'

$au = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU'
New-Item -Path $au -Force | Out-Null
New-ItemProperty -Path $au -Name 'NoAutoUpdate'            -PropertyType DWord -Value 1 -Force | Out-Null
New-ItemProperty -Path $au -Name 'AUOptions'               -PropertyType DWord -Value 1 -Force | Out-Null
New-ItemProperty -Path $au -Name 'ScheduledInstallDay'     -PropertyType DWord -Value 0 -Force | Out-Null
New-ItemProperty -Path $au -Name 'NoAutoRebootWithLoggedOnUsers' -PropertyType DWord -Value 1 -Force | Out-Null

$wu = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate'
New-Item -Path $wu -Force | Out-Null
New-ItemProperty -Path $wu -Name 'DoNotConnectToWindowsUpdateInternetLocations' -PropertyType DWord -Value 1 -Force | Out-Null

foreach ($svc in 'wuauserv','UsoSvc','WaaSMedicSvc') {
    $s = Get-Service -Name $svc -ErrorAction SilentlyContinue
    if ($s) {
        Set-Service -Name $svc -StartupType Disabled -ErrorAction SilentlyContinue
        Stop-Service -Name $svc -Force -ErrorAction SilentlyContinue
    }
}
