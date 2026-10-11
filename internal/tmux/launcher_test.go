package tmux

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestLauncherTargetsClientPaneAndPreservesPaths(t *testing.T) {
	var commands [][]string
	launcher := Launcher{
		clientName: "client",
		lookPath:   func(string) (string, error) { return "/tools/agent's bin/codex", nil },
		runCommand: func(_ context.Context, command string, args ...string) ([]byte, error) {
			commands = append(commands, append([]string{command}, args...))
			return []byte("%42\n"), nil
		},
	}
	directory := t.TempDir()
	if err := launcher.Launch(context.Background(), "codex", directory, false); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{
		{"display-message", "-c", "client", "-p", "#{pane_id}"},
		{"split-window", "-t", "%42", "-c", directory, shellQuote("/tools/agent's bin/codex")},
	}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("commands: %#v", commands)
	}
}

func TestLauncherDoesNotCreatePaneForInvalidRequests(t *testing.T) {
	launcher := Launcher{
		lookPath: func(string) (string, error) { return "", errors.New("missing") },
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("created pane for invalid request")
			return nil, nil
		},
	}
	requests := []struct{ provider, directory string }{
		{"other", t.TempDir()},
		{"codex", "/nonexistent/a-gent"},
		{"claude", t.TempDir()},
	}
	for _, request := range requests {
		if err := launcher.Launch(context.Background(), request.provider, request.directory, false); err == nil {
			t.Fatal("expected launch error")
		}
	}
}

func TestLauncherCreatesWindowInOriginatingSession(t *testing.T) {
	var commands [][]string
	launcher := Launcher{
		clientName: "client",
		lookPath:   func(string) (string, error) { return "/tools/claude", nil },
		runCommand: func(_ context.Context, command string, args ...string) ([]byte, error) {
			commands = append(commands, append([]string{command}, args...))
			return []byte("$3\n"), nil
		},
	}
	directory := t.TempDir()
	if err := launcher.Launch(context.Background(), "claude", directory, true); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{
		{"display-message", "-c", "client", "-p", "#{session_id}"},
		{"new-window", "-t", "$3:", "-c", directory, shellQuote("/tools/claude")},
	}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("commands: %#v", commands)
	}
}

func TestOpenCodeLauncherEnablesLocalStatusAPI(t *testing.T) {
	var commands [][]string
	launcher := Launcher{
		clientName: "client",
		lookPath: func(provider string) (string, error) {
			if provider != "opencode" {
				t.Fatalf("wrong provider: %s", provider)
			}
			return "/tools/agent's bin/opencode", nil
		},
		runCommand: func(_ context.Context, command string, args ...string) ([]byte, error) {
			commands = append(commands, append([]string{command}, args...))
			return []byte("$3\n"), nil
		},
	}
	directory := t.TempDir()
	if err := launcher.Launch(context.Background(), "opencode", directory, true); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{
		{"display-message", "-c", "client", "-p", "#{session_id}"},
		{"new-window", "-t", "$3:", "-c", directory, shellQuote("/tools/agent's bin/opencode") + " --port 0 --hostname 127.0.0.1"},
	}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("commands: %#v", commands)
	}
}

func TestPiLauncherLoadsObserverAndCreatesNewWindow(t *testing.T) {
	var commands [][]string
	launcher := Launcher{
		clientName: "client",
		lookPath:   func(string) (string, error) { return "/tools/pi", nil },
		piObserver: func() (string, error) { return "/cache/agent's observer.ts", nil },
		runCommand: func(_ context.Context, command string, args ...string) ([]byte, error) {
			commands = append(commands, append([]string{command}, args...))
			return []byte("$3\n"), nil
		},
	}
	directory := t.TempDir()
	if err := launcher.Launch(context.Background(), "pi", directory, true); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{
		{"display-message", "-c", "client", "-p", "#{session_id}"},
		{"new-window", "-t", "$3:", "-c", directory,
			shellQuote("/tools/pi") + " --extension " + shellQuote("/cache/agent's observer.ts")},
	}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("commands: %#v", commands)
	}
	commands = nil
	launcher.piObserver = func() (string, error) { return "", errors.New("unwritable cache") }
	if err := launcher.Launch(context.Background(), "pi", directory, true); err == nil {
		t.Fatal("ignored observer error")
	}
	if len(commands) != 1 {
		t.Fatal("launched Pi without observable status")
	}
}

func TestHermesLauncherLoadsObserverAndCreatesNewWindow(t *testing.T) {
	var commands [][]string
	launcher := Launcher{
		clientName:     "client",
		lookPath:       func(string) (string, error) { return "/tools/hermes", nil },
		hermesObserver: func() (string, error) { return "/home/agent's plugin/__init__.py", nil },
		runCommand: func(_ context.Context, command string, args ...string) ([]byte, error) {
			commands = append(commands, append([]string{command}, args...))
			return []byte("$3\n"), nil
		},
	}
	directory := t.TempDir()
	if err := launcher.Launch(context.Background(), "hermes", directory, true); err != nil {
		t.Fatal(err)
	}
	expected := [][]string{
		{"display-message", "-c", "client", "-p", "#{session_id}"},
		{"new-window", "-t", "$3:", "-c", directory,
			shellQuote("/tools/hermes")},
	}
	if !reflect.DeepEqual(commands, expected) {
		t.Fatalf("commands: %#v", commands)
	}
	commands = nil
	launcher.hermesObserver = func() (string, error) { return "", errors.New("unwritable cache") }
	if err := launcher.Launch(context.Background(), "hermes", directory, true); err == nil {
		t.Fatal("ignored observer error")
	}
	if len(commands) != 1 {
		t.Fatal("launched Hermes without observable status")
	}
}
