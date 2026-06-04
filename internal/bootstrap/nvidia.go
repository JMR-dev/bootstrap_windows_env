package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var (
	fetchNvidiaDriverURLFn      = fetchNvidiaDriverURL
	getInstalledNvidiaVersionFn = getInstalledNvidiaVersion
)

type AjaxDriverResponse struct {
	IDS []struct {
		DownloadInfo struct {
			DownloadURL string `json:"DownloadURL"`
			Version     string `json:"Version"`
		} `json:"downloadInfo"`
	} `json:"IDS"`
}

// resolveNvidiaPFID looks up the GPU model in the embedded gpu-data.json and returns its pfid.
func resolveNvidiaPFID(model string) (string, error) {
	data, err := managedAssets.ReadFile("assets/gpu-data.json")
	if err != nil {
		return "", fmt.Errorf("read embedded gpu-data.json: %w", err)
	}

	var gpuData struct {
		Desktop  map[string]string `json:"desktop"`
		Notebook map[string]string `json:"notebook"`
	}
	if err := json.Unmarshal(data, &gpuData); err != nil {
		return "", fmt.Errorf("unmarshal gpu-data.json: %w", err)
	}

	normalized := strings.TrimSpace(model)
	if !strings.HasPrefix(strings.ToLower(normalized), "geforce") {
		normalized = "GeForce " + normalized
	}

	for k, v := range gpuData.Desktop {
		if strings.EqualFold(k, normalized) {
			return v, nil
		}
	}
	for k, v := range gpuData.Notebook {
		if strings.EqualFold(k, normalized) {
			return v, nil
		}
	}

	return "", fmt.Errorf("Nvidia model %q not found in database", model)
}

// fetchNvidiaDriverURL queries the Nvidia driver lookup API and returns the download URL and version.
func fetchNvidiaDriverURL(pfid string, driverType string) (string, string, error) {
	// driverType is either "Game Ready" (upCRD=0) or "Studio" (upCRD=1)
	upCRD := "0"
	if driverType == "Studio" {
		upCRD = "1"
	}
	apiURL := fmt.Sprintf("https://gfwsl.geforce.com/services_toolkit/services/com/nvidia/services/AjaxDriverService.php?func=DriverManualLookup&pfid=%s&osID=57&upCRD=%s&dch=1", pfid, upCRD)

	resp, err := http.Get(apiURL)
	if err != nil {
		return "", "", fmt.Errorf("http get driver info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("driver info request returned status %d", resp.StatusCode)
	}

	var data AjaxDriverResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", fmt.Errorf("decode driver info: %w", err)
	}

	if len(data.IDS) == 0 {
		return "", "", fmt.Errorf("no drivers found for pfid %s", pfid)
	}

	downloadURL := data.IDS[0].DownloadInfo.DownloadURL
	version := data.IDS[0].DownloadInfo.Version
	if downloadURL == "" {
		return "", "", fmt.Errorf("download URL not found in driver info response")
	}

	return downloadURL, version, nil
}

// getInstalledNvidiaVersion queries the current system's Nvidia driver version.
func getInstalledNvidiaVersion(ctx context.Context, runner Runner) (string, error) {
	cmd := "Get-CimInstance Win32_VideoController | Where-Object { $_.Name -like '*Nvidia*' } | Select-Object -ExpandProperty DriverVersion"
	result := runner.Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", cmd)
	if result.Err != nil {
		return "", result.Err
	}
	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		return "", nil // No Nvidia GPU or driver installed
	}
	lines := strings.Split(output, "\n")
	driverVer := strings.TrimSpace(lines[0])
	if driverVer == "" {
		return "", nil
	}

	parts := strings.Split(driverVer, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected driver version format: %s", driverVer)
	}
	combined := parts[len(parts)-2] + parts[len(parts)-1]
	if len(combined) < 5 {
		return "", fmt.Errorf("unexpected combined driver version suffix: %s", combined)
	}
	suffix5 := combined[len(combined)-5:]
	nvidiaVer := fmt.Sprintf("%s.%s", suffix5[:3], suffix5[3:])
	nvidiaVer = strings.TrimPrefix(nvidiaVer, "0")
	return nvidiaVer, nil
}

// isRTX2050OrNewer returns true if the GPU model is an RTX 2050 or newer.
// Since RTX started with Turing architecture (20-series) and RTX 2050 is the lowest number in the RTX branding,
// containing "RTX" (case-insensitive) is a robust and future-proof check.
func isRTX2050OrNewer(model string) bool {
	return strings.Contains(strings.ToUpper(model), "RTX")
}
