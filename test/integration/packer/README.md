# Vagrant box build (Packer + Hyper-V + Windows 11 25H2)

Builds a `bootstrap-win11.box` Vagrant box from a stock Windows 11 25H2 ISO,
ready to use with `test/integration/Vagrantfile`.

## Prerequisites

On the host (must be Windows with Hyper-V):

- Hyper-V role enabled; current user in **Hyper-V Administrators**.
- [HashiCorp Packer](https://developer.hashicorp.com/packer/install) 1.15+ in
  `PATH`. HashiCorp does not publish a winget manifest, so use the helper:

  ```powershell
  # from an elevated PowerShell
  cd test\integration\packer
  .\install-packer.ps1                 # installs the pinned default version
  .\install-packer.ps1 -Version 1.14.0 # or pick a specific release
  ```

  The script downloads
  `https://releases.hashicorp.com/packer/<Version>/packer_<Version>_windows_amd64.zip`,
  extracts `packer.exe` to `C:\Program Files\Packer\`, and adds that directory
  to the machine-scoped `PATH`. It is idempotent — re-running with the same
  version just re-asserts the `PATH` entry. See the `param()` block in
  `install-packer.ps1` for available flags.

  By default the same script also installs **Windows ADK Deployment Tools**
  to get `oscdimg.exe`, which Packer's `hyperv-iso` builder needs to build the
  unattended-install secondary ISO from `cd_files`. Only the
  `OptionId.DeploymentTools` feature is selected, so the install is small
  (~70 MB) and unattended. Pass `-SkipOscdimg` to opt out (e.g. if you already
  have `xorriso`/`mkisofs` on `PATH`). After install you may need to open a
  fresh shell — or run
  `$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine')` — for
  `oscdimg.exe` to resolve in the current session.
- [HashiCorp Vagrant](https://developer.hashicorp.com/vagrant/install) 2.4+ in
  `PATH` (used for the `vagrant` post-processor and, later, the integration
  test itself).
- A Windows 11 client ISO somewhere on disk. Its path is read from `.env`
  (see below) and is **not** hardcoded in the Packer config.
- Roughly 90 GB free disk and ~60 minutes of patience for a first build.
- An elevated PowerShell session for the build (`packer build` shells out to
  Hyper-V management APIs that require admin).

## Configure the ISO path via `.env`

The Packer config has no default ISO path. Copy the template, fill it in, and
load it into the current shell before building:

```powershell
cd test\integration\packer
Copy-Item .env.example .env
# edit .env so PKR_VAR_iso_path points at your Windows 11 ISO
. .\load-env.ps1
```

`load-env.ps1` reads `.env` line-by-line and exports each `KEY=VALUE` into the
process environment. `.env` is gitignored at the repo root, so your local path
never gets committed.

Packer picks the values up automatically through its `PKR_VAR_<name>` env-var
convention; nothing about the `packer build` invocation changes.

## One-time setup

```powershell
packer init .\windows-11.pkr.hcl
```

That downloads the `hyperv` and `vagrant` Packer plugins.

## Compute and pin the ISO checksum (recommended)

```powershell
Get-FileHash $env:PKR_VAR_iso_path -Algorithm SHA256
```

Set `PKR_VAR_iso_checksum=sha256:<digest>` in `.env` and re-run `load-env.ps1`.
Leaving it as `none` skips validation; fine for local builds but loses the "my
ISO has not been swapped" guarantee.

## Build

```powershell
packer build .\windows-11.pkr.hcl
```

Roughly what happens:

1. Packer creates a Gen 2 Hyper-V VM (Secure Boot + TPM + 24 vCPU + 32 GB RAM
   + 80 GB dynamic VHDX) attached to `Default Switch`. The build VM is
   throwaway and intentionally claims most of the host for speed; the
   runtime integration VM uses smaller defaults set in
   `test/integration/Vagrantfile` (8 vCPU / 16 GB).
2. It mounts the Windows ISO and a tiny `PROVISION` ISO containing
   `cd/autounattend.xml` and `cd/scripts/oobe-enable-winrm.ps1`.
3. Windows Setup performs an unattended install (Pro edition, generic
   activation key) and creates user `vagrant`:`vagrant`.
4. FirstLogonCommands enable WinRM (HTTP, basic auth) and set
   `LocalAccountTokenFilterPolicy=1` so remote admin tokens are not filtered.
5. Packer connects over WinRM and runs the provisioners in `scripts/`:
   - `configure-os.ps1`         — UAC token policy, DiagTrack off, NTP resync.
   - `disable-windows-updates.ps1` — fully disables WU and friends.
   - `disable-defender-cloud.ps1`  — disables MAPS / cloud lookups.
   - `install-vagrant-key.ps1`     — drops the vagrant insecure public key.
   - `compact.ps1`                 — cleans caches, zeros free space.
6. Packer shuts the guest down cleanly and the `vagrant` post-processor
   packages the VHDX into `bootstrap-win11.box`.

## Register the resulting box with Vagrant

```powershell
vagrant box add bootstrap-win11 .\bootstrap-win11.box
```

Then in `test/integration/Vagrantfile` set:

```powershell
$env:BOOTSTRAP_BOX = "bootstrap-win11"
go run ..\..\test\integration
```

(or just edit the default in the Vagrantfile to `bootstrap-win11`).

## Layout

```
test/integration/packer/
├── README.md                   ← this file
├── .env.example                ← template; copy to .env (gitignored)
├── load-env.ps1                ← dot-source to export PKR_VAR_* into shell
├── install-packer.ps1          ← elevated installer for packer.exe
├── windows-11.pkr.hcl          ← Packer HCL2 build definition
├── vagrantfile-template.rb     ← baked into the output box
├── cd/
│   ├── autounattend.xml        ← Windows Setup answer file
│   └── scripts/
│       └── oobe-enable-winrm.ps1
└── scripts/                    ← provisioners run after WinRM is up
    ├── configure-os.ps1
    ├── disable-windows-updates.ps1
    ├── disable-defender-cloud.ps1
    ├── install-vagrant-key.ps1
    └── compact.ps1
```

## Notes and gotchas

- **Run elevated.** `packer build` opens Hyper-V management APIs; non-elevated
  shells fail with confusing "access denied" messages midway through.
- **First build is slow.** Windows Setup + updates trimming + cipher /w on a
  60 GB volume can take 45–90 min. Subsequent rebuilds reuse the parent VHDX
  via linked clone and are much faster.
- **License compliance.** The box uses the public KMS client setup key
  (W269N-WFGWX-YVC9B-4J6C9-T83GX) to pick the Pro edition during install. The
  resulting VM is not activated; for short-lived integration runs the eval
  period is more than sufficient, but redistribute the box only under your
  own licensing terms.
- **25H2 OOBE.** The autounattend sets `BypassNRO=1` during `specialize`
  because 24H2/25H2 removed the `BypassNRO.cmd` helper. If a future Windows
  update changes the OOBE flow again, the symptom will be a hung
  `vagrant up` waiting for WinRM; check by opening Hyper-V Manager and
  looking for an OOBE screen on the VM console.
- **Secure Boot.** Enabled (`MicrosoftWindows` template). Switch off in the
  Packer source if you ever need to install unsigned kernel-mode drivers in
  the box.
