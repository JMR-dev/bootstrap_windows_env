package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCustomActionsNoAIStillKeepsPlaywrightAndExtension(t *testing.T) {
	actions := CustomActions(Options{NoAI: true})
	names := make(map[string]bool)
	for _, action := range actions {
		if action.AI {
			t.Fatalf("AI action was included with --no-ai: %s", action.Name)
		}
		names[action.Name] = true
	}
	for _, want := range []string{"Playwright CLI", "GitHub CLI gh-repo-bootstrap extension"} {
		if !names[want] {
			t.Fatalf("missing non-AI action %s", want)
		}
	}
	if names["agy"] {
		t.Fatal("agy should be skipped with --no-ai")
	}
}

func TestRuntimeAndCustomDependencyOrdering(t *testing.T) {
	host := HostActions()
	if len(host) != 4 || host[0].Name != "Node.js LTS through fnm" || host[1].Name != "pyenv-win through Scoop or Chocolatey" || host[2].Name != "latest stable Python through pyenv-win" || host[3].Name != "VLC default media player associations" {
		t.Fatalf("unexpected host action order: %#v", host)
	}
	pyenvInstall := strings.Join(host[1].Commands[0].Args, " ")
	for _, want := range []string{"scoop", "Invoke-Native 'choco' @('install', 'pyenv-win'", "Chocolatey.Chocolatey"} {
		if !strings.Contains(pyenvInstall, want) {
			t.Fatalf("pyenv install command missing %q: %s", want, pyenvInstall)
		}
	}
	custom := CustomActions(Options{})
	if len(custom) == 0 || custom[0].Name != "agy" {
		t.Fatalf("unexpected custom action order: %#v", custom)
	}
	foundPlaywright := false
	foundAI := false
	for _, action := range custom {
		if action.Name == "Playwright CLI" {
			foundPlaywright = true
		}
		if action.AI {
			foundAI = true
		}
	}
	if !foundPlaywright || !foundAI {
		t.Fatalf("custom actions missing playwright or AI actions: %#v", custom)
	}
}

func TestVLCDefaultMediaActionCoversCommonAudioVideoExtensions(t *testing.T) {
	host := HostActions()
	vlc := host[len(host)-1]
	script := strings.Join(vlc.Commands[0].Args, " ")
	for _, want := range []string{
		"VideoLAN\\VLC\\vlc.exe",
		"VlcDefaultAssociations.xml",
		"/Import-DefaultAppAssociations",
		".mp3",
		".flac",
		".mp4",
		".mkv",
		".avi",
		".webm",
		".wmv",
		"vlc-default-media.done",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("VLC default media action missing %q", want)
		}
	}
}

func TestPendingActionsFiltersInstalledActions(t *testing.T) {
	actions := []Action{
		{Name: "done"},
		{Name: "pending"},
	}
	pending := PendingActions([]ActionState{
		{Action: actions[0], Installed: true},
		{Action: actions[1]},
	})
	if len(pending) != 1 || pending[0].Name != "pending" {
		t.Fatalf("pending = %#v, want only pending action", pending)
	}
}

func TestExecuteActionsRunsCommandsAndStopsCurrentActionOnFailure(t *testing.T) {
	first := CommandSpec{Name: "first", Args: []string{"ok"}}
	fail := CommandSpec{Name: "second", Args: []string{"fail"}}
	skipped := CommandSpec{Name: "third", Args: []string{"skipped"}}
	runner := &fakeRunner{responses: map[string]CommandResult{
		commandKey(fail.Name, fail.Args...): {Err: errors.New("boom"), Stderr: "failed"},
	}}
	issues := ExecuteActions(context.Background(), runner, []Action{{
		Name:     "compound action",
		Commands: []CommandSpec{first, fail, skipped},
	}})
	if len(issues) != 1 || !strings.Contains(issues[0].Err.Error(), "failed") {
		t.Fatalf("issues = %#v, want second command failure", issues)
	}
	if !runner.called(first.Name, first.Args...) || !runner.called(fail.Name, fail.Args...) {
		t.Fatal("expected first and failing commands to run")
	}
	if runner.called(skipped.Name, skipped.Args...) {
		t.Fatal("commands after a failing command should not run for the same action")
	}
}

func TestExecuteActionsReturnsNoIssuesOnSuccess(t *testing.T) {
	command := CommandSpec{Name: "ok"}
	issues := ExecuteActions(context.Background(), &fakeRunner{}, []Action{{
		Name:     "successful action",
		Commands: []CommandSpec{command},
	}})
	if err := issues.Err(); err != nil {
		t.Fatal(err)
	}
}
