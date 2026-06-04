# Turn off Defender cloud-delivered protection and sample submission.
#
# Cloud lookups add tens of seconds per winget install on a cold box and have
# no value in an ephemeral integration VM.

$ErrorActionPreference = 'Continue'

Set-MpPreference -MAPSReporting Disabled                 -ErrorAction SilentlyContinue
Set-MpPreference -SubmitSamplesConsent NeverSend         -ErrorAction SilentlyContinue
Set-MpPreference -DisableBlockAtFirstSeen $true          -ErrorAction SilentlyContinue
Set-MpPreference -DisableIOAVProtection $true            -ErrorAction SilentlyContinue
Set-MpPreference -ScanAvgCPULoadFactor 5                 -ErrorAction SilentlyContinue
