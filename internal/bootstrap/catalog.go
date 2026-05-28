package bootstrap

type Classification string

const (
	ClassHeadless Classification = "headless"
	ClassGUI      Classification = "gui"
)

type RebootBehavior string

const (
	RebootNone     RebootBehavior = "none"
	RebootPossible RebootBehavior = "possible"
)

type Package struct {
	Name              string
	Phase             Phase
	WingetID          string
	WingetSource      string
	WingetOverride    string
	WingetConfigAsset string
	Class             Classification
	RequiresElevation bool
	Reboot            RebootBehavior
	PostInstall       string
	AI                bool
}

func wingetPackage(name, id string, phase Phase, class Classification) Package {
	return Package{
		Name:         name,
		Phase:        phase,
		WingetID:     id,
		WingetSource: "winget",
		Class:        class,
		Reboot:       RebootNone,
	}
}

// Catalog is deliberately explicit: unlisted software is not installed by a fallback provider.
func Catalog() []Package {
	packages := []Package{
		{
			Name:              "Visual Studio Community 2026",
			Phase:             PhaseHost,
			WingetID:          "Microsoft.VisualStudio.Community",
			WingetSource:      "winget",
			WingetOverride:    "--passive --config {config}",
			WingetConfigAsset: "assets/visual-studio-community.vsconfig",
			Class:             ClassGUI,
			RequiresElevation: true,
			Reboot:            RebootPossible,
			PostInstall:       "install workloads/components from managed .vsconfig; use Microsoft C/C++ compilers and CMake",
		},
		wingetPackage("WezTerm", "wez.wezterm", PhaseHost, ClassGUI),
		wingetPackage("PowerShell 7", "Microsoft.PowerShell", PhaseHost, ClassHeadless),
		wingetPackage("PowerToys", "Microsoft.PowerToys", PhaseHost, ClassGUI),
		wingetPackage("JetBrainsMono Nerd Font", "DEVCOM.JetBrainsMonoNerdFont", PhaseHost, ClassHeadless),
		wingetPackage("Android Studio", "Google.AndroidStudio", PhaseHost, ClassGUI),
		wingetPackage("FilePilot", "Voidstar.FilePilot", PhaseHost, ClassGUI),
		wingetPackage("GitHub Desktop", "GitHub.GitHubDesktop", PhaseHost, ClassGUI),
		wingetPackage("Google Chrome", "Google.Chrome", PhaseHost, ClassGUI),
		wingetPackage("Vivaldi", "Vivaldi.Vivaldi", PhaseHost, ClassGUI),
		wingetPackage("OBS Studio", "OBSProject.OBSStudio", PhaseHost, ClassGUI),
		wingetPackage("Obsidian", "Obsidian.Obsidian", PhaseHost, ClassGUI),
		wingetPackage("Libre Hardware Monitor", "LibreHardwareMonitor.LibreHardwareMonitor", PhaseHost, ClassGUI),
		wingetPackage("Mullvad VPN", "MullvadVPN.MullvadVPN", PhaseHost, ClassGUI),
		wingetPackage("WireGuard", "WireGuard.WireGuard", PhaseHost, ClassGUI),
		wingetPackage("Steam", "Valve.Steam", PhaseHost, ClassGUI),
		wingetPackage("Bruno", "Bruno.Bruno", PhaseHost, ClassGUI),
		wingetPackage("Figma", "Figma.Figma", PhaseHost, ClassGUI),
		wingetPackage("VLC", "VideoLAN.VLC", PhaseHost, ClassGUI),
		wingetPackage("Wireshark", "WiresharkFoundation.Wireshark", PhaseHost, ClassGUI),
		wingetPackage("Zoom", "Zoom.Zoom", PhaseHost, ClassGUI),
		wingetPackage("HandBrake", "HandBrake.HandBrake", PhaseHost, ClassGUI),
		wingetPackage("Adobe Acrobat Reader", "Adobe.Acrobat.Reader.64-bit", PhaseHost, ClassGUI),
		wingetPackage("Gpg4win (includes gpgOL)", "GnuPG.Gpg4win", PhaseHost, ClassGUI),
		wingetPackage("Tor Browser", "TorProject.TorBrowser", PhaseHost, ClassGUI),
		wingetPackage("LibreWolf", "LibreWolf.LibreWolf", PhaseHost, ClassGUI),
		wingetPackage("Rufus", "Rufus.Rufus", PhaseHost, ClassGUI),
		wingetPackage("Slack", "SlackTechnologies.Slack", PhaseHost, ClassGUI),
		wingetPackage("Discord", "Discord.Discord", PhaseHost, ClassGUI),
		wingetPackage("Revo Uninstaller", "RevoUninstaller.RevoUninstaller", PhaseHost, ClassGUI),
		wingetPackage("darktable", "darktable.darktable", PhaseHost, ClassGUI),
		wingetPackage("Floorp", "Ablaze.Floorp", PhaseHost, ClassGUI),
		wingetPackage("Git", "Git.Git", PhaseHost, ClassHeadless),
		wingetPackage("GitHub CLI", "GitHub.cli", PhaseHost, ClassHeadless),
		wingetPackage("Neovim", "Neovim.Neovim", PhaseHost, ClassHeadless),
		wingetPackage("ripgrep", "BurntSushi.ripgrep.MSVC", PhaseHost, ClassHeadless),
		wingetPackage("FFmpeg", "Gyan.FFmpeg", PhaseHost, ClassHeadless),
		wingetPackage("lazygit", "JesseDuffield.lazygit", PhaseHost, ClassHeadless),
		wingetPackage("Lua", "DEVCOM.Lua", PhaseHost, ClassHeadless),
		wingetPackage("minisign", "jedisct1.minisign", PhaseHost, ClassHeadless),
		wingetPackage("Google Cloud CLI", "Google.CloudSDK", PhaseHost, ClassHeadless),
		wingetPackage("Rustup", "Rustlang.Rustup", PhaseHost, ClassHeadless),
		wingetPackage("AWS CLI", "Amazon.AWSCLI", PhaseHost, ClassHeadless),
		wingetPackage("Azure CLI", "Microsoft.AzureCLI", PhaseHost, ClassHeadless),
		wingetPackage("Pulumi", "Pulumi.Pulumi", PhaseHost, ClassHeadless),
		wingetPackage("restic", "restic.restic", PhaseHost, ClassHeadless),
		wingetPackage("yt-dlp", "yt-dlp.yt-dlp", PhaseHost, ClassHeadless),
		wingetPackage("Vagrant", "Hashicorp.Vagrant", PhaseHost, ClassHeadless),
		wingetPackage("Go", "GoLang.Go", PhaseHost, ClassHeadless),
		wingetPackage(".NET SDK", "Microsoft.DotNet.SDK.10", PhaseHost, ClassHeadless),
		wingetPackage("Temurin JDK", "EclipseAdoptium.Temurin.21.JDK", PhaseHost, ClassHeadless),
		wingetPackage("Zig", "zig.zig", PhaseHost, ClassHeadless),
		wingetPackage("CMake", "Kitware.CMake", PhaseHost, ClassHeadless),
		wingetPackage("fnm", "Schniz.fnm", PhaseHost, ClassHeadless),
		wingetPackage("oh-my-posh", "JanDeDobbeleer.OhMyPosh", PhaseCustom, ClassHeadless),
	}
	for i := range packages {
		if packages[i].Name == "Vagrant" {
			packages[i].RequiresElevation = true
			packages[i].Reboot = RebootPossible
			packages[i].PostInstall = "configure Hyper-V provider prerequisites"
		}
		if packages[i].Name == "fnm" {
			packages[i].PostInstall = "install Node.js LTS"
		}
	}
	return packages
}

func PackagesForPhase(opts Options, phase Phase) []Package {
	var selected []Package
	for _, pkg := range Catalog() {
		if pkg.Phase != phase || (opts.NoAI && pkg.AI) {
			continue
		}
		selected = append(selected, pkg)
	}
	return selected
}

// DeferredWindowsTools records intentionally unsupported native workloads in v1.
var DeferredWindowsTools = []string{
	"Webcamoid (no current exact winget manifest)",
	"Docker Desktop", "Podman Desktop", "Buildah", "Minikube", "Firecracker",
	"QEMU", "virt-manager", "Windows Terminal", "Visual Studio Code",
}
