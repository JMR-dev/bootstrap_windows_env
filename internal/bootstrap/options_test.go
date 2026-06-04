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

func TestNvidiaOptions(t *testing.T) {
	// Valid combinations
	opts, err := ParseOptions([]string{"--nvidia", "--model", "RTX 4090", "--type", "Studio"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Nvidia || opts.NvidiaModel != "RTX 4090" || opts.NvidiaType != "Studio" {
		t.Fatal("Nvidia options not parsed correctly")
	}

	opts, err = ParseOptions([]string{"--nvidia", "--model", "RTX 4090", "--type", "Game Ready"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Nvidia || opts.NvidiaModel != "RTX 4090" || opts.NvidiaType != "Game Ready" {
		t.Fatal("Nvidia options not parsed correctly")
	}

	// Missing model
	if _, err := ParseOptions([]string{"--nvidia", "--type", "Studio"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when model is missing")
	}

	// Missing type
	if _, err := ParseOptions([]string{"--nvidia", "--model", "RTX 4090"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when type is missing")
	}

	// Invalid type
	if _, err := ParseOptions([]string{"--nvidia", "--model", "RTX 4090", "--type", "Other"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error with invalid type")
	}

	// Subflags passed without --nvidia
	if _, err := ParseOptions([]string{"--model", "RTX 4090"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when --model passed without --nvidia")
	}
	if _, err := ParseOptions([]string{"--type", "Studio"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when --type passed without --nvidia")
	}

	// AMD options
	opts, err = ParseOptions([]string{"--amd"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Amd {
		t.Fatal("AMD option not parsed correctly")
	}

	// AMD and Nvidia together (should succeed)
	opts, err = ParseOptions([]string{"--nvidia", "--model", "RTX 4090", "--type", "Studio", "--amd"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Nvidia || opts.NvidiaModel != "RTX 4090" || opts.NvidiaType != "Studio" || !opts.Amd {
		t.Fatal("AMD and Nvidia options combined not parsed correctly")
	}

	// AMD, Nvidia and Intel conflict (all three should fail by default)
	if _, err := ParseOptions([]string{"--nvidia", "--model", "RTX 4090", "--type", "Studio", "--amd", "--intel"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when --nvidia, --amd, and --intel are all passed")
	}

	// AMD, Nvidia and Intel combined with BOOTSTRAP_INTEGRATION_TEST=true (should succeed)
	t.Setenv("BOOTSTRAP_INTEGRATION_TEST", "true")
	opts, err = ParseOptions([]string{"--nvidia", "--model", "RTX 4090", "--type", "Studio", "--amd", "--intel"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Nvidia || !opts.Amd || !opts.Intel {
		t.Fatal("All three options combined not parsed correctly under integration test context")
	}

	// Intel options
	opts, err = ParseOptions([]string{"--intel"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Intel {
		t.Fatal("Intel option not parsed correctly")
	}
}
