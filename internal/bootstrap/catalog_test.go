package bootstrap

import "testing"

func TestCatalogContainsExactChocoIDsAndDefersExcludedNativeTools(t *testing.T) {
	seen := make(map[string]Package)
	for _, pkg := range Catalog() {
		if pkg.ChocoID == "" && !isCustomPackage(pkg) {
			t.Fatalf("invalid choco metadata for %#v", pkg)
		}
		seen[pkg.Name] = pkg
	}
	for _, name := range []string{
		"Android Studio",
		"FilePilot",
		"PowerShell 7",
		"Libre Hardware Monitor",
		"Mullvad VPN",
		"WireGuard",
		"Steam",
		"Bruno",
		"Figma",
		"VLC",
		"HandBrake",
		"Adobe Acrobat Reader",
		"Gpg4win (includes gpgOL)",
		"Tor Browser",
		"LibreWolf",
		"Google Cloud CLI",
		"Rustup",
		"AWS CLI",
		"Azure CLI",
		"Rufus",
		"Slack",
		"Discord",
		"Revo Uninstaller",
		"lazygit",
		"CMake",
		"WezTerm",
		"Vagrant",
		"oh-my-posh",
		"Nmap",
		"Krita",
		"Godot C++",
		"Blender",
		"WhatsApp",
		"Affinity",
		"Tidal",
		"DaVinci Resolve",
		"Ardour",
	} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("missing catalog package %s", name)
		}
	}
	for _, deferred := range DeferredWindowsTools {
		if _, ok := seen[deferred]; ok {
			t.Fatalf("%s should be deferred, not installed natively", deferred)
		}
	}
	if !seen["Vagrant"].RequiresElevation || seen["Vagrant"].PostInstall == "" {
		t.Fatal("Vagrant should carry Hyper-V/elevation metadata")
	}
	if _, ok := seen["pyenv-win"]; ok {
		t.Fatal("pyenv-win is installed through managed host actions, not the winget catalog")
	}
	if _, ok := seen["gcc"]; ok {
		t.Fatal("gcc should not be installed natively; use Visual Studio C/C++ tools")
	}
	if _, ok := seen["g++"]; ok {
		t.Fatal("g++ should not be installed natively; use Visual Studio C/C++ tools")
	}
}
