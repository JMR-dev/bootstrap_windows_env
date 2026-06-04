packer {
  required_plugins {
    hyperv = {
      version = ">= 1.1.4"
      source  = "github.com/hashicorp/hyperv"
    }
    vagrant = {
      version = ">= 1.1.5"
      source  = "github.com/hashicorp/vagrant"
    }
  }
}

# ─── Inputs ───────────────────────────────────────────────────────────────────

variable "iso_path" {
  type        = string
  description = "Absolute path to the Windows 11 installer ISO. Set via PKR_VAR_iso_path (load .env first; see README)."
}

variable "iso_checksum" {
  type        = string
  description = "Checksum of the ISO (e.g. 'sha256:<hex>'); 'none' skips validation. Set via PKR_VAR_iso_checksum."
  default     = "none"
}

variable "vm_name" {
  type    = string
  default = "bootstrap-win11"
}

variable "output_directory" {
  type    = string
  default = "output-bootstrap-win11"
}

variable "box_output" {
  type    = string
  default = "bootstrap-win11.box"
}

variable "cpus" {
  type        = number
  default     = 24
  description = "vCPUs allocated to the build VM. The Packer guest is throwaway and gets the full host budget for speed; the runtime integration VM keeps a smaller footprint in test/integration/Vagrantfile."
}

variable "memory" {
  type        = number
  default     = 32768
  description = "Build-VM memory (MB). Throwaway VM, full host budget."
}

variable "disk_size" {
  type        = number
  default     = 81920
  description = "Guest disk size in MB."
}

variable "switch_name" {
  type    = string
  default = "Default Switch"
}

variable "winrm_username" {
  type    = string
  default = "vagrant"
}

variable "winrm_password" {
  type      = string
  default   = "vagrant"
  sensitive = true
}

# ─── Source ───────────────────────────────────────────────────────────────────
#
# Hyper-V Gen 2 + Secure Boot is required for modern Windows 11. The
# unattend payload lives on a small ISO that Packer builds on the fly from
# `cd_files` and attaches as a secondary drive; Windows Setup auto-discovers
# `autounattend.xml` on any attached media at the root.

source "hyperv-iso" "win11" {
  vm_name      = var.vm_name
  iso_url      = var.iso_path
  iso_checksum = var.iso_checksum

  cpus      = var.cpus
  memory    = var.memory
  disk_size = var.disk_size

  generation                       = 2
  enable_secure_boot               = true
  secure_boot_template             = "MicrosoftWindows"
  enable_tpm                       = true
  enable_dynamic_memory            = false
  enable_virtualization_extensions = false
  guest_additions_mode             = "disable"
  switch_name                      = var.switch_name

  output_directory = var.output_directory

  communicator   = "winrm"
  winrm_username = var.winrm_username
  winrm_password = var.winrm_password
  winrm_timeout  = "2h"
  winrm_use_ssl  = false
  winrm_insecure = true

  # Gen 2 UEFI shows "Press any key to boot from CD or DVD..." for ~5s.
  # Default boot_wait is 10s (already past the prompt) and default
  # boot_command is empty, so without these the firmware falls through to
  # PXE and Windows Setup never starts. Hammer ENTER early to catch it.
  boot_wait    = "1s"
  boot_command = ["<enter><wait><enter><wait><enter>"]

  shutdown_command = "shutdown /s /t 10 /f /d p:4:1 /c \"Packer Shutdown\""
  shutdown_timeout = "30m"

  # Build a tiny "PROVISION" ISO containing the unattend file. The
  # FirstLogonCommands in autounattend.xml enable WinRM inline (no helper
  # script needed) since Windows Setup may detach secondary DVDs after
  # install, leaving the PROVISION volume unreachable at first logon.
  cd_files = [
    "./cd/autounattend.xml",
  ]
  cd_label = "PROVISION"
}

# ─── Build ────────────────────────────────────────────────────────────────────
#
# Provisioners run *after* WinRM is reachable, which is after OOBE has finished
# and the autounattend's FirstLogonCommands have brought WinRM online. Each
# script is small and single-purpose; see comments in each .ps1.

build {
  sources = ["source.hyperv-iso.win11"]

  provisioner "powershell" {
    elevated_user     = var.winrm_username
    elevated_password = var.winrm_password
    scripts = [
      "./scripts/configure-os.ps1",
      "./scripts/disable-windows-updates.ps1",
      "./scripts/disable-defender-cloud.ps1",
      "./scripts/install-vagrant-key.ps1",
      "./scripts/compact.ps1",
    ]
  }

  post-processor "vagrant" {
    output              = var.box_output
    keep_input_artifact = false
    provider_override   = "hyperv"
    vagrantfile_template = "./vagrantfile-template.rb"
  }
}
