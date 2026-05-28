package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerCapturesOutputAndTimeout(t *testing.T) {
	result := ExecRunner{}.Run(context.Background(), "cmd.exe", "/C", "echo ok")
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Stdout, "ok") || !strings.Contains(result.Command, "cmd.exe /C echo ok") {
		t.Fatalf("result = %#v", result)
	}

	result = ExecRunner{Timeout: time.Millisecond}.Run(context.Background(), "powershell.exe", "-NoLogo", "-NoProfile", "-Command", "Start-Sleep -Seconds 5")
	if result.Err == nil || !strings.Contains(result.Err.Error(), "timed out") {
		t.Fatalf("result error = %v, want timeout", result.Err)
	}
}
