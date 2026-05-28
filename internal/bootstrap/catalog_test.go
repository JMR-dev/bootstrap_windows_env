package bootstrap

import "testing"

func TestCatalogContainsExactWingetIDsAndDefersExcludedNativeTools(t *testing.T) {
	seen := make(map[string]Package)
	for _, pkg := range Catalog() {
		if pkg.WingetID == "" || pkg.WingetSource != "winget" {
			t.Fatalf("invalid winget metadata for %#v", pkg)
		}
		seen[pkg.Name] = pkg
	}
	for _, name := range []string{
		"Visual Studio Community 2026",
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
		"fnm",
		"oh-my-posh",
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
	vs := seen["Visual Studio Community 2026"]
	if vs.WingetID != "Microsoft.VisualStudio.Community" || vs.WingetConfigAsset == "" || vs.WingetOverride == "" {
		t.Fatalf("Visual Studio package missing managed config metadata: %#v", vs)
	}
}
