# Integration test (Vagrant + Hyper-V)

`bootstrap_windows_env` runs against Windows 10/11 client SKUs only — Windows
Server (which is what GitHub-hosted runners provide) does not support several
of the features the bootstrapper exercises (System Restore checkpoints,
Microsoft Store package updates, the Chris Titus Tech WinUtil flows, etc.).
The integration test therefore runs locally on a Windows host against a real
Windows client virtual machine through Hyper-V, driven by Vagrant.

## Prerequisites

On the host machine:

- Windows 10/11 Pro or Enterprise (or Windows Server) with the **Hyper-V**
  role enabled.
- The host user must be a member of the **Hyper-V Administrators** local
  group.
- [HashiCorp Vagrant](https://developer.hashicorp.com/vagrant/downloads)
  2.4 or newer in `PATH`.
- Go (the same toolchain version pinned in `go.mod`) — used both to build the
  bootstrapper and to run the integration driver.
- A Hyper-V-compatible Windows client Vagrant box. The default is
  [`gusztavvargadr/windows-11`](https://portal.cloud.hashicorp.com/vagrant/discover/gusztavvargadr/windows-11),
  which ships with WinRM enabled and the standard `vagrant`/`vagrant`
  credentials. To build your own from a Windows 11 ISO, see
  [`packer/README.md`](packer/README.md) and then set
  `BOOTSTRAP_BOX=bootstrap-win11` (or edit the Vagrantfile default).

## Running the test

From the repository root:

```powershell
go run ./test/integration
```

That will:

1. Cross-build `dist/bootstrap_windows_env.exe` for `windows/amd64`.
2. `vagrant up --provider=hyperv` against `test/integration/Vagrantfile`.
3. `vagrant upload` the binary into the guest at `C:\Users\vagrant\bootstrap_windows_env.exe`.
4. Execute the binary inside the guest via `vagrant powershell -c "& ... --yes --no-wsl"`.
5. `vagrant destroy -f` regardless of pass/fail (use `--keep` to preserve the VM).

## Flags

| Flag           | Default                       | Description                                                                  |
|----------------|-------------------------------|------------------------------------------------------------------------------|
| `--args`       | `--yes --no-wsl`              | Arguments passed to `bootstrap_windows_env.exe` inside the guest.            |
| `--keep`       | `false`                       | Leave the VM running after the test for debugging.                           |
| `--skip-build` | `false`                       | Reuse `dist/bootstrap_windows_env.exe` instead of rebuilding it.             |
| `--provider`   | `hyperv`                      | Vagrant provider name (only `hyperv` is supported).                          |
| `--timeout`    | `90m`                         | Overall timeout. The Vagrantfile has its own WinRM/boot timeouts on top.     |

## Vagrantfile environment overrides

The Vagrantfile reads these environment variables so you can tune the VM
without editing the file:

| Variable                  | Default                       | Description                                  |
|---------------------------|-------------------------------|----------------------------------------------|
| `BOOTSTRAP_BOX`           | `gusztavvargadr/windows-11`   | Vagrant box reference.                       |
| `BOOTSTRAP_BOX_VERSION`   | _(unset)_                     | Pin a specific box version.                  |
| `BOOTSTRAP_HOSTNAME`      | `bootstrap-it`                | Guest hostname.                              |
| `BOOTSTRAP_CPUS`          | `8`                           | Guest vCPU count.                            |
| `BOOTSTRAP_MEMORY`        | `16384`                       | Guest startup memory (MB).                   |
| `BOOTSTRAP_MAX_MEMORY`    | `16384`                       | Guest dynamic-memory ceiling (MB).           |
| `BOOTSTRAP_BOOT_TIMEOUT`  | `1800`                        | Vagrant boot timeout (seconds).              |
| `BOOTSTRAP_WINRM_USER`    | `vagrant`                     | Guest WinRM user.                            |
| `BOOTSTRAP_WINRM_PASS`    | `vagrant`                     | Guest WinRM password.                        |
| `BOOTSTRAP_WINRM_TIMEOUT` | `3600`                        | Per-WinRM-command timeout (seconds).         |
| `BOOTSTRAP_WINRM_RETRIES` | `30`                          | WinRM retry attempts during `vagrant up`.    |

## Notes

- There is no official Vagrant SDK for Go — Vagrant is a Ruby project — so the
  driver shells out to the `vagrant` CLI. This is the conventional pattern for
  Go programs orchestrating external tools.
- The Hyper-V provider cannot reliably share folders without prompting for SMB
  credentials, so the driver pushes the bootstrapper into the guest with
  `vagrant upload` and disables the default synced folder.
- This test is intentionally **not** wired up to GitHub Actions: hosted runners
  do not expose Hyper-V and run Windows Server, which the bootstrapper does
  not target. Run it on a workstation, lab box, or self-hosted Windows client
  runner.
