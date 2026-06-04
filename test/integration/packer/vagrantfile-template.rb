# Vagrantfile fragment baked into the output .box file by the
# `packer-post-processor-vagrant` plugin. This is what every `vagrant up`
# of a derived box inherits before our top-level Vagrantfile is applied.

Vagrant.configure("2") do |config|
  config.vm.communicator   = "winrm"
  config.vm.guest          = :windows
  config.winrm.username    = "vagrant"
  config.winrm.password    = "vagrant"
  config.winrm.transport   = :plaintext
  config.winrm.basic_auth_only = true
  config.vm.boot_timeout   = 1800

  config.vm.provider "hyperv" do |hv|
    hv.enable_virtualization_extensions = false
    hv.linked_clone = true
    hv.vm_integration_services = {
      guest_service_interface: true,
      heartbeat:               true,
      shutdown:                true,
      time_synchronization:    true,
      vss:                     true,
    }
  end
end
