package bootstrap

import (
	"flag"
	"fmt"
	"io"
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
	Headless         bool
	LinuxReleaseRepo string
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
	fs.BoolVar(&opts.Headless, "headless", false, "disable interactive login offers and require --yes to execute")
	fs.StringVar(&opts.LinuxReleaseRepo, "linux-release-repo", "JMR-dev/bootstrap_dev_env", "GitHub repository containing Linux bootstrap releases")
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
	if opts.Headless && !opts.Yes && !opts.DryRun {
		return Options{}, fmt.Errorf("--headless execution requires --yes or --dry-run")
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
