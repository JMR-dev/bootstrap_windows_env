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
	ChocoID           string
	ChocoParams       string
	InstallerArgs     string
	ChocoConfigAsset  string
	Class             Classification
	RequiresElevation bool
	Reboot            RebootBehavior
	PostInstall       string
	AI                bool
}

func chocoPackage(name, id string, phase Phase, class Classification) Package {
	return Package{
		Name:    name,
		Phase:   phase,
		ChocoID: id,
		Class:   class,
		Reboot:  RebootNone,
	}
}

// Catalog is deliberately explicit: unlisted software is not installed by a fallback provider.
func Catalog() []Package {
	packages := []Package{
		chocoPackage("WezTerm", "wezterm", PhaseHost, ClassGUI),
		chocoPackage("PowerShell 7", "powershell-core", PhaseHost, ClassHeadless),
		chocoPackage("PowerToys", "powertoys", PhaseHost, ClassGUI),
		chocoPackage("JetBrainsMono Nerd Font", "nerd-fonts-jetbrainsmono", PhaseHost, ClassHeadless),
		chocoPackage("Android Studio", "androidstudio", PhaseHost, ClassGUI),
		// FilePilot has no official Chocolatey package, but it's listed here.
		// We use empty ChocoID so our custom installer downloads and installs it directly.
		chocoPackage("FilePilot", "", PhaseHost, ClassGUI),
		chocoPackage("Vivaldi", "vivaldi", PhaseHost, ClassGUI),
		chocoPackage("OBS Studio", "obs-studio", PhaseHost, ClassGUI),
		chocoPackage("Obsidian", "obsidian", PhaseHost, ClassGUI),
		chocoPackage("Libre Hardware Monitor", "librehardwaremonitor", PhaseHost, ClassGUI),
		chocoPackage("WireGuard", "wireguard", PhaseHost, ClassGUI),
		chocoPackage("Steam", "steam", PhaseHost, ClassGUI),
		chocoPackage("Bruno", "bruno", PhaseHost, ClassGUI),
		chocoPackage("VLC", "vlc", PhaseHost, ClassGUI),
		chocoPackage("Wireshark", "wireshark", PhaseHost, ClassGUI),
		chocoPackage("HandBrake", "handbrake", PhaseHost, ClassGUI),
		chocoPackage("Gpg4win (includes gpgOL)", "gpg4win", PhaseHost, ClassGUI),
		chocoPackage("Tor Browser", "tor-browser", PhaseHost, ClassGUI),
		chocoPackage("LibreWolf", "librewolf", PhaseHost, ClassGUI),
		chocoPackage("Rufus", "rufus", PhaseHost, ClassGUI),
		chocoPackage("Slack", "slack", PhaseHost, ClassGUI),
		chocoPackage("Discord", "discord", PhaseHost, ClassGUI),
		chocoPackage("Revo Uninstaller", "revo-uninstaller", PhaseHost, ClassGUI),
		chocoPackage("darktable", "darktable", PhaseHost, ClassGUI),
		chocoPackage("Floorp", "floorp", PhaseHost, ClassGUI),
		chocoPackage("Krita", "krita", PhaseHost, ClassGUI),
		chocoPackage("Godot C++", "godot", PhaseHost, ClassGUI),
		chocoPackage("Blender", "blender", PhaseHost, ClassGUI),
		chocoPackage("Git", "git", PhaseHost, ClassHeadless),
		chocoPackage("GitHub CLI", "gh", PhaseHost, ClassHeadless),
		chocoPackage("Neovim", "neovim", PhaseHost, ClassHeadless),
		chocoPackage("trivy", "trivy", PhaseHost, ClassHeadless),
		chocoPackage("cosign", "cosign", PhaseHost, ClassHeadless),
		chocoPackage("gitleaks", "gitleaks", PhaseHost, ClassHeadless),
		chocoPackage("jq", "jq", PhaseHost, ClassHeadless),
		chocoPackage("yq", "yq", PhaseHost, ClassHeadless),
		chocoPackage("fzf", "fzf", PhaseHost, ClassHeadless),
		chocoPackage("fd", "fd", PhaseHost, ClassHeadless),
		chocoPackage("bottom", "bottom", PhaseHost, ClassHeadless),
		chocoPackage("ripgrep", "ripgrep", PhaseHost, ClassHeadless),
		chocoPackage("FFmpeg", "ffmpeg", PhaseHost, ClassHeadless),
		chocoPackage("lazygit", "lazygit", PhaseHost, ClassHeadless),
		chocoPackage("Lua", "lua", PhaseHost, ClassHeadless),
		chocoPackage("minisign", "minisign", PhaseHost, ClassHeadless),
		chocoPackage("Google Cloud CLI", "gcloudsdk", PhaseHost, ClassHeadless),
		chocoPackage("Rustup", "rustup.install", PhaseHost, ClassHeadless),
		chocoPackage("AWS CLI", "awscli", PhaseHost, ClassHeadless),
		chocoPackage("Azure CLI", "azure-cli", PhaseHost, ClassHeadless),
		chocoPackage("Pulumi", "pulumi", PhaseHost, ClassHeadless),
		chocoPackage("restic", "restic", PhaseHost, ClassHeadless),
		chocoPackage("yt-dlp", "yt-dlp", PhaseHost, ClassHeadless),
		chocoPackage("Vagrant", "vagrant", PhaseHost, ClassHeadless),
		chocoPackage("Go", "golang", PhaseHost, ClassHeadless),
		chocoPackage(".NET SDK", "dotnet-sdk", PhaseHost, ClassHeadless),
		chocoPackage("Temurin JDK", "temurin", PhaseHost, ClassHeadless),
		chocoPackage("Zig", "zig", PhaseHost, ClassHeadless),
		chocoPackage("CMake", "cmake", PhaseHost, ClassHeadless),
		chocoPackage("oh-my-posh", "oh-my-posh", PhaseCustom, ClassHeadless),
		chocoPackage("Google Chrome", "googlechrome", PhaseCustom, ClassGUI),
		chocoPackage("Mullvad VPN", "mullvad-app", PhaseCustom, ClassGUI),
		chocoPackage("Figma", "figma", PhaseCustom, ClassGUI),
		chocoPackage("Zoom", "zoom", PhaseCustom, ClassGUI),
		chocoPackage("Adobe Acrobat Reader", "adobereader", PhaseCustom, ClassGUI),
		chocoPackage("WhatsApp", "", PhaseCustom, ClassGUI),
		chocoPackage("Affinity", "", PhaseCustom, ClassGUI),
		chocoPackage("Tidal", "", PhaseCustom, ClassGUI),
		chocoPackage("DaVinci Resolve", "", PhaseCustom, ClassGUI),
		chocoPackage("Ardour", "", PhaseCustom, ClassGUI),
		chocoPackage("Nvidia Driver", "", PhaseHost, ClassGUI),
		chocoPackage("Nvidia Broadcast", "", PhaseHost, ClassGUI),
		chocoPackage("AMD Auto-Detect", "", PhaseHost, ClassGUI),
		chocoPackage("Intel DSA", "", PhaseHost, ClassGUI),
		Package{
			Name:          "Nmap",
			Phase:         PhaseCustom,
			ChocoID:       "nmap",
			InstallerArgs: `"/S /NPCAP=NO"`,
			Class:         ClassHeadless,
			Reboot:        RebootNone,
		},
	}
	for i := range packages {
		if packages[i].Name == "Vagrant" {
			packages[i].RequiresElevation = true
			packages[i].Reboot = RebootPossible
			packages[i].PostInstall = "configure Hyper-V provider prerequisites"
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
		if pkg.Name == "Nvidia Driver" {
			if !opts.Nvidia {
				continue
			}
			pkg.ChocoParams = opts.NvidiaModel
			pkg.InstallerArgs = opts.NvidiaType
		}
		if pkg.Name == "Nvidia Broadcast" {
			if !opts.Nvidia || !isRTX2050OrNewer(opts.NvidiaModel) {
				continue
			}
		}
		if pkg.Name == "AMD Auto-Detect" {
			if !opts.Amd {
				continue
			}
		}
		if pkg.Name == "Intel DSA" {
			if !opts.Intel {
				continue
			}
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
