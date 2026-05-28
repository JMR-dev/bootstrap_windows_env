package bootstrap

import (
	"bytes"
	"testing"
)

func TestSelectedPhasesHonorsOnlyAndNoWSL(t *testing.T) {
	defaultPhases := SelectedPhases(Options{})
	if len(defaultPhases) == 0 || defaultPhases[0] != PhaseOS {
		t.Fatalf("default phases = %v, want OS phase first", defaultPhases)
	}

	opts, err := ParseOptions([]string{"--only", "custom", "--no-ai"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	phases := SelectedPhases(opts)
	if len(phases) != 1 || phases[0] != PhaseCustom {
		t.Fatalf("phases = %v, want custom only", phases)
	}
	if !opts.NoAI {
		t.Fatal("--no-ai was not parsed")
	}

	opts, err = ParseOptions([]string{"--no-wsl"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range SelectedPhases(opts) {
		if phase == PhaseWSL {
			t.Fatal("WSL phase was selected with --no-wsl")
		}
	}
}

func TestInvalidOptionCombinations(t *testing.T) {
	if _, err := ParseOptions([]string{"--only", "wsl", "--no-wsl"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected --only wsl --no-wsl to fail")
	}
	if _, err := ParseOptions([]string{"--headless"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected --headless without --yes or --dry-run to fail")
	}
	if _, err := ParseOptions([]string{"--only", "bogus"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid --only value to fail")
	}
	if _, err := ParseOptions([]string{"unexpected"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected unexpected positional argument to fail")
	}
	if _, err := ParseOptions([]string{"--not-a-real-flag"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected flag parse failure")
	}
}

func TestParseOptionsAcceptsHeadlessWithDryRunAndCustomRepo(t *testing.T) {
	opts, err := ParseOptions([]string{"--headless", "--dry-run", "--linux-release-repo", "owner/repo"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Headless || !opts.DryRun || opts.LinuxReleaseRepo != "owner/repo" {
		t.Fatalf("opts = %#v", opts)
	}
}

func TestParseOptionsAcceptsNoRestorePoints(t *testing.T) {
	opts, err := ParseOptions([]string{"--yes", "--no-restore-points"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.NoRestorePoints {
		t.Fatalf("expected NoRestorePoints to be set: %#v", opts)
	}
	defaults, err := ParseOptions(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.NoRestorePoints {
		t.Fatal("NoRestorePoints should default to false")
	}
}
