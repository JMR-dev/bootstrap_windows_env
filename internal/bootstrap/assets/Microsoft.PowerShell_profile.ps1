# Managed by bootstrap_windows_env. Local additions belong in Microsoft.PowerShell_profile.local.ps1.
$ompConfig = Join-Path $HOME '.config\oh-my-posh\jmr.omp.json'
if (Get-Command oh-my-posh -ErrorAction SilentlyContinue) {
    oh-my-posh init pwsh --config $ompConfig | Invoke-Expression
}

if (Get-Command fnm -ErrorAction SilentlyContinue) {
    fnm env --use-on-cd --shell powershell | Out-String | Invoke-Expression
}

$pyenvRoots = @(
    $env:PYENV,
    (Join-Path $HOME '.pyenv\pyenv-win'),
    (Join-Path $HOME 'scoop\apps\pyenv\current\pyenv-win')
) | Where-Object { $_ } | Select-Object -Unique
foreach ($root in $pyenvRoots) {
    $pyenvBin = Join-Path $root 'bin'
    $pyenvShims = Join-Path $root 'shims'
    if ((Test-Path $pyenvBin) -and ($env:Path -notlike "*$pyenvBin*")) {
        $env:PYENV = $root
        $env:PYENV_ROOT = $root
        $env:PYENV_HOME = $root
        $env:Path = "$pyenvBin;$pyenvShims;$env:Path"
        break
    }
}

$localProfile = Join-Path (Split-Path -Parent $PROFILE) 'Microsoft.PowerShell_profile.local.ps1'
if (Test-Path $localProfile) {
    . $localProfile
}
