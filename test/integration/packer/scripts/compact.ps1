# Zero out free space so the resulting .box file is small.
#
# Runs last, just before Packer initiates shutdown. Cleans caches and the WU
# component store, which directly shrinks the on-disk footprint of the VHDX.
# We intentionally do not call `cipher /w:C:\` here: it costs 15–30 minutes
# and its only payoff is better tarball compression of unused space, which
# only matters when shipping the box. Re-add it before publishing publicly.

$ErrorActionPreference = 'Continue'

# Clean Windows Update download cache and component store.
Stop-Service -Name wuauserv -Force -ErrorAction SilentlyContinue
Remove-Item  -Path "$env:WINDIR\SoftwareDistribution\Download\*" -Recurse -Force -ErrorAction SilentlyContinue
& dism /Online /Cleanup-Image /StartComponentCleanup /ResetBase /Quiet

# Empty Recycle Bin and temp dirs.
Get-ChildItem -Path "$env:TEMP" -Force -ErrorAction SilentlyContinue |
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
Clear-RecycleBin -Force -ErrorAction SilentlyContinue

# Light defrag pass to consolidate free extents. Quick on an SSD-backed VHDX.
& defrag C: /U /V

# Drop hibernation file (Windows 11 has it on by default).
& powercfg /h off
