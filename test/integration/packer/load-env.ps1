# Loads variables from .env (in this directory) into the current PowerShell
# session as environment variables. Dot-source it so the variables stick:
#
#     . .\load-env.ps1
#
# Format:
#   - One KEY=VALUE per line.
#   - Lines starting with '#' and blank lines are ignored.
#   - Values are taken verbatim (no shell quoting / interpolation). Surround
#     with quotes only if you actually want quotes in the value.

$ErrorActionPreference = 'Stop'

$envFile = Join-Path $PSScriptRoot '.env'
if (-not (Test-Path -LiteralPath $envFile)) {
    throw ".env not found at $envFile. Copy .env.example to .env and fill in PKR_VAR_iso_path."
}

$loaded = 0
foreach ($line in Get-Content -LiteralPath $envFile) {
    $trimmed = $line.Trim()
    if ($trimmed -eq '' -or $trimmed.StartsWith('#')) { continue }
    $idx = $trimmed.IndexOf('=')
    if ($idx -lt 1) {
        Write-Warning "skipping malformed line: $line"
        continue
    }
    $name  = $trimmed.Substring(0, $idx).Trim()
    $value = $trimmed.Substring($idx + 1)
    [Environment]::SetEnvironmentVariable($name, $value, 'Process')
    $loaded++
}

Write-Host "loaded $loaded variable(s) from $envFile into the current session."
