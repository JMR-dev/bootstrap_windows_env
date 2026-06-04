package bootstrap

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// Phase identifies a resumable unit of Windows provisioning work.
type Phase string

const (
	PhaseOS     Phase = "os"
	PhaseHost   Phase = "host"
	PhaseCustom Phase = "custom"
	PhaseConfig Phase = "config"
	PhaseWSL    Phase = "wsl"
)

var PhaseOrder = []Phase{PhaseOS, PhaseHost, PhaseCustom, PhaseConfig, PhaseWSL}

type Options struct {
	Only             Phase
	NoAI             bool
	NoWSL            bool
	Yes              bool
	DryRun           bool
	LinuxReleaseRepo string
	Nvidia           bool
	NvidiaModel      string
	NvidiaType       string
	Amd              bool
	Intel            bool
}

func ParseOptions(args []string, stderr io.Writer) (Options, error) {
	var opts Options
	var only string
	fs := flag.NewFlagSet("bootstrap_windows_env", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&only, "only", "", "run one phase: os, host, custom, config, or wsl")
	fs.BoolVar(&opts.NoAI, "no-ai", false, "omit native AI CLIs and pass --no-ai to the Fedora guest")
	fs.BoolVar(&opts.NoWSL, "no-wsl", false, "omit Fedora WSL provisioning and guest bootstrap")
	fs.BoolVar(&opts.Yes, "yes", false, "execute the displayed plan without confirmation")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "detect state and display work without making changes")
	fs.StringVar(&opts.LinuxReleaseRepo, "linux-release-repo", "JMR-dev/bootstrap_dev_env", "GitHub repository containing Linux bootstrap releases")
	fs.BoolVar(&opts.Nvidia, "nvidia", false, "fetch and install Nvidia driver")
	fs.StringVar(&opts.NvidiaModel, "model", "", "Nvidia GPU model (e.g. RTX 4090)")
	fs.StringVar(&opts.NvidiaType, "type", "", "Nvidia driver type (Game Ready or Studio)")
	fs.BoolVar(&opts.Amd, "amd", false, "fetch and run AMD auto-detection graphics driver tool")
	fs.BoolVar(&opts.Intel, "intel", false, "fetch and install Intel Driver & Support Assistant")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if fs.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if only != "" {
		opts.Only = Phase(only)
		switch opts.Only {
		case PhaseOS, PhaseHost, PhaseCustom, PhaseConfig, PhaseWSL:
		default:
			return Options{}, fmt.Errorf("invalid --only value %q: use os, host, custom, config, or wsl", only)
		}
	}
	if opts.NoWSL && opts.Only == PhaseWSL {
		return Options{}, fmt.Errorf("--only wsl cannot be combined with --no-wsl")
	}
	if opts.Nvidia && opts.Amd && opts.Intel && os.Getenv("BOOTSTRAP_INTEGRATION_TEST") != "true" {
		return Options{}, fmt.Errorf("cannot combine --nvidia, --amd, and --intel")
	}
	if opts.Nvidia {
		if opts.NvidiaModel == "" {
			return Options{}, fmt.Errorf("--nvidia requires --model flag")
		}
		if opts.NvidiaType == "" {
			return Options{}, fmt.Errorf("--nvidia requires --type flag")
		}
		if opts.NvidiaType != "Game Ready" && opts.NvidiaType != "Studio" {
			return Options{}, fmt.Errorf("invalid --type value %q: use Game Ready or Studio", opts.NvidiaType)
		}
	} else if opts.NvidiaModel != "" || opts.NvidiaType != "" {
		return Options{}, fmt.Errorf("--model and --type require --nvidia flag")
	}
	return opts, nil
}

func (o Options) Includes(phase Phase) bool {
	if o.NoWSL && phase == PhaseWSL {
		return false
	}
	return o.Only == "" || o.Only == phase
}

func SelectedPhases(opts Options) []Phase {
	phases := make([]Phase, 0, len(PhaseOrder))
	for _, phase := range PhaseOrder {
		if opts.Includes(phase) {
			phases = append(phases, phase)
		}
	}
	return phases
}
