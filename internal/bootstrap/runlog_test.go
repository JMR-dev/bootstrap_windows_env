package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRunLogWritesResultsToDocuments(t *testing.T) {
	documents := filepath.Join(t.TempDir(), "Documents")
	report := NewRunReport(Options{Yes: true, NoWSL: true}, []Phase{PhaseOS, PhaseHost})
	report.Event("phase os started")
	report.Notice("reboot required later")
	report.Issue(Issue{Step: "apply power settings", Err: errors.New("powercfg failed")})
	report.Finish(report.Issues.Err())

	path, err := WriteRunLog(UserPaths{Documents: documents}, report)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != documents {
		t.Fatalf("log path = %s, want under %s", path, documents)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"result: failed", "phase os started", "reboot required later", "apply power settings: powercfg failed"} {
		if !strings.Contains(content, want) {
			t.Fatalf("log missing %q:\n%s", want, content)
		}
	}
}

func TestWriteRunLogHandlesUnfinishedReportAndEmptyDocumentsPath(t *testing.T) {
	documents := filepath.Join(t.TempDir(), "Documents")
	report := NewRunReport(Options{}, []Phase{PhaseOS})
	path, err := WriteRunLog(UserPaths{Documents: documents}, report)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "finished:") || !strings.Contains(content, "result: completed") {
		t.Fatalf("unexpected unfinished log content:\n%s", content)
	}

	if _, err := WriteRunLog(UserPaths{}, report); err == nil {
		t.Fatal("expected empty documents path error")
	}
}

func TestWriteRunLogReportsWriteFailure(t *testing.T) {
	documents := filepath.Join(t.TempDir(), "Documents")
	report := NewRunReport(Options{}, []Phase{PhaseOS})
	report.Started = report.Started.UTC().Truncate(0)
	logName := "bootstrap_windows_env-" + report.Started.Format("20060102-150405") + ".log"
	if err := os.MkdirAll(filepath.Join(documents, logName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRunLog(UserPaths{Documents: documents}, report); err == nil {
		t.Fatal("expected write failure when log path is a directory")
	}
}
