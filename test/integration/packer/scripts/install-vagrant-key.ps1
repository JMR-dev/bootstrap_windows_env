# Install the Vagrant insecure SSH key marker so `vagrant ssh-config` and
# tooling that expects the Vagrant-flavoured user account is happy. WinRM is
# the actual communicator, but several Vagrant plugins probe for this layout.

$ErrorActionPreference = 'Continue'

$sshDir = "$env:USERPROFILE\.ssh"
New-Item -ItemType Directory -Path $sshDir -Force | Out-Null

$keyUrl = 'https://raw.githubusercontent.com/hashicorp/vagrant/main/keys/vagrant.pub'
try {
    Invoke-WebRequest -UseBasicParsing -Uri $keyUrl -OutFile "$sshDir\authorized_keys"
} catch {
    Write-Host "warning: could not fetch vagrant insecure public key: $($_.Exception.Message)"
}
