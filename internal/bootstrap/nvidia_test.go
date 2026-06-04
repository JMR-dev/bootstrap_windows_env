package bootstrap

import (
	"context"
	"strings"
	"testing"
)

func TestResolveNvidiaPFID(t *testing.T) {
	tests := []struct {
		model    string
		wantPFID string
		wantErr  bool
	}{
		{"RTX 4090", "995", false},
		{"GeForce RTX 4090", "995", false},
		{"rtx 4090", "995", false},
		{"RTX 4090 D", "1036", false},
		{"RTX 4090 Laptop GPU", "1004", false},
		{"NonExistentGPU", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, err := resolveNvidiaPFID(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveNvidiaPFID(%q) error = %v, wantErr %v", tt.model, err, tt.wantErr)
				return
			}
			if got != tt.wantPFID {
				t.Errorf("resolveNvidiaPFID(%q) = %q, want %q", tt.model, got, tt.wantPFID)
			}
		})
	}
}

func TestFetchNvidiaDriverURL(t *testing.T) {
	// Query API for RTX 4090 Game Ready
	url, version, err := fetchNvidiaDriverURL("995", "Game Ready")
	if err != nil {
		t.Skipf("Skipping API test (possibly offline or rate limited): %v", err)
		return
	}
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("expected download URL to start with https://, got %q", url)
	}
	if version == "" {
		t.Error("expected version to be non-empty")
	}

	// Query API for RTX 4090 Studio
	urlStudio, versionStudio, err := fetchNvidiaDriverURL("995", "Studio")
	if err != nil {
		t.Skipf("Skipping API test: %v", err)
		return
	}
	if !strings.HasPrefix(urlStudio, "https://") {
		t.Errorf("expected studio download URL to start with https://, got %q", urlStudio)
	}
	if versionStudio == "" {
		t.Error("expected studio version to be non-empty")
	}
}

func TestGetInstalledNvidiaVersion(t *testing.T) {
	// Test with a mock runner that returns a valid driver version format
	mockRunner := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Get-CimInstance Win32_VideoController | Where-Object { $_.Name -like '*Nvidia*' } | Select-Object -ExpandProperty DriverVersion"): {
				Stdout: "32.0.15.9636\n",
			},
		},
	}

	ver, err := getInstalledNvidiaVersion(context.Background(), mockRunner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != "596.36" {
		t.Errorf("getInstalledNvidiaVersion = %q, want %q", ver, "596.36")
	}

	// Test case with leading zero representation
	mockRunner2 := &fakeRunner{
		responses: map[string]CommandResult{
			commandKey("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "Get-CimInstance Win32_VideoController | Where-Object { $_.Name -like '*Nvidia*' } | Select-Object -ExpandProperty DriverVersion"): {
				Stdout: "32.0.10.6104\n",
			},
		},
	}
	ver2, err := getInstalledNvidiaVersion(context.Background(), mockRunner2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver2 != "61.04" {
		t.Errorf("getInstalledNvidiaVersion = %q, want %q", ver2, "61.04")
	}
}
