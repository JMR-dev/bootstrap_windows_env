package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RunReport struct {
	Started    time.Time
	Finished   time.Time
	Options    Options
	Phases     []Phase
	Events     []string
	Notices    []string
	Issues     Issues
	FinalError string
}

func NewRunReport(opts Options, phases []Phase) *RunReport {
	return &RunReport{
		Started: time.Now(),
		Options: opts,
		Phases:  append([]Phase(nil), phases...),
	}
}

func (r *RunReport) Event(format string, args ...any) {
	r.Events = append(r.Events, fmt.Sprintf(format, args...))
}

func (r *RunReport) Notice(notice string) {
	r.Notices = append(r.Notices, notice)
	r.Event("notice: %s", notice)
}

func (r *RunReport) Issue(issue Issue) {
	r.Issues = append(r.Issues, issue)
	r.Event("error: %s: %v", issue.Step, issue.Err)
}

func (r *RunReport) Finish(err error) {
	r.Finished = time.Now()
	if err != nil {
		r.FinalError = err.Error()
	}
}

func WriteRunLog(paths UserPaths, report *RunReport) (string, error) {
	documents := paths.Documents
	if documents == "" {
		return "", fmt.Errorf("documents path is empty")
	}
	if err := os.MkdirAll(documents, 0o755); err != nil {
		return "", err
	}
	name := "bootstrap_windows_env-" + report.Started.Format("20060102-150405") + ".log"
	path := filepath.Join(documents, name)
	var b strings.Builder
	fmt.Fprintf(&b, "bootstrap_windows_env run\n")
	fmt.Fprintf(&b, "started: %s\n", report.Started.Format(time.RFC3339))
	if !report.Finished.IsZero() {
		fmt.Fprintf(&b, "finished: %s\n", report.Finished.Format(time.RFC3339))
		fmt.Fprintf(&b, "duration: %s\n", report.Finished.Sub(report.Started).Round(time.Second))
	}
	fmt.Fprintf(&b, "phases: %s\n", phasesForLog(report.Phases))
	fmt.Fprintf(&b, "options: only=%s no-ai=%t no-wsl=%t yes=%t dry-run=%t headless=%t linux-release-repo=%s\n",
		report.Options.Only,
		report.Options.NoAI,
		report.Options.NoWSL,
		report.Options.Yes,
		report.Options.DryRun,
		report.Options.Headless,
		report.Options.LinuxReleaseRepo,
	)
	if report.FinalError != "" {
		fmt.Fprintf(&b, "result: failed\n")
		fmt.Fprintf(&b, "error: %s\n", report.FinalError)
	} else {
		fmt.Fprintf(&b, "result: completed\n")
	}
	if len(report.Events) > 0 {
		fmt.Fprintf(&b, "\nevents:\n")
		for _, event := range report.Events {
			fmt.Fprintf(&b, "- %s\n", event)
		}
	}
	if len(report.Notices) > 0 {
		fmt.Fprintf(&b, "\nnotices:\n")
		for _, notice := range report.Notices {
			fmt.Fprintf(&b, "- %s\n", notice)
		}
	}
	if len(report.Issues) > 0 {
		fmt.Fprintf(&b, "\nerrors:\n")
		for _, issue := range report.Issues {
			fmt.Fprintf(&b, "- %s: %v\n", issue.Step, issue.Err)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func phasesForLog(phases []Phase) string {
	parts := make([]string, 0, len(phases))
	for _, phase := range phases {
		parts = append(parts, string(phase))
	}
	return strings.Join(parts, ",")
}
