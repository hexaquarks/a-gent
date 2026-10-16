package tmux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestNewLauncherRequiresTmuxAndAnOriginatingClient(t *testing.T) {
	for _, test := range []struct {
		name         string
		tmux         string
		client       string
		clientOutput string
		commandExit  string
		wantClient   string
	}{
		{name: "outside tmux", client: "preserved client"},
		{name: "preserved client", tmux: "fixture", client: "preserved client", wantClient: "preserved client"},
		{name: "active client", tmux: "fixture", clientOutput: "  active client\n", wantClient: "active client"},
		{name: "no active client", tmux: "fixture", clientOutput: " \n"},
		{name: "client lookup fails", tmux: "fixture", commandExit: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TMUX", test.tmux)
			t.Setenv(tmuxClientEnvironmentVariable, test.client)
			t.Setenv("CLIENT_OUTPUT", test.clientOutput)
			commandExit := test.commandExit
			if commandExit == "" {
				commandExit = "0"
			}
			t.Setenv("COMMAND_EXIT", commandExit)

			argumentsPath := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("COMMAND_ARGUMENTS", argumentsPath)
			isolateCommand(t, "tmux", `printf '%s\0' "$@" > "$COMMAND_ARGUMENTS"
printf '%s' "$CLIENT_OUTPUT"
exit "$COMMAND_EXIT"
`)

			launcher := NewLauncher()
			if test.wantClient == "" {
				if launcher != nil {
					t.Fatalf("launcher = %#v, want nil", launcher)
				}
			} else if launcher == nil || launcher.clientName != test.wantClient {
				t.Fatalf("launcher = %#v, want client %q", launcher, test.wantClient)
			}

			if test.tmux == "" || test.client != "" {
				if _, err := os.Stat(argumentsPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected tmux lookup: %v", err)
				}
			} else {
				wantArguments := []string{"display-message", "-p", "#{client_name}"}
				if got := recordedArguments(t, argumentsPath); !reflect.DeepEqual(got, wantArguments) {
					t.Fatalf("client lookup arguments = %#v, want %#v", got, wantArguments)
				}
			}
		})
	}
}

func TestLauncherRejectsFileAsProjectDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project-file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	launcher := Launcher{
		lookPath: func(string) (string, error) {
			t.Fatal("looked up provider for a file")
			return "", nil
		},
	}
	err := launcher.Launch(context.Background(), "codex", path, false)
	if err == nil || err.Error() != "project path is not a directory" {
		t.Fatalf("launch error = %v", err)
	}
}

func TestLauncherReportsWorkspaceAndLaunchFailures(t *testing.T) {
	commandError := errors.New("tmux unavailable")
	for _, test := range []struct {
		name         string
		workspace    string
		lookupError  error
		launchError  error
		newWindow    bool
		wantMessage  string
		wantCommands int
	}{
		{
			name: "workspace lookup fails", lookupError: commandError,
			wantMessage: "find tmux workspace", wantCommands: 1,
		},
		{
			name: "empty workspace", workspace: " \n",
			wantMessage: "no tmux workspace found", wantCommands: 1,
		},
		{
			name: "split fails", workspace: "%42", launchError: commandError,
			wantMessage: "start codex in tmux pane", wantCommands: 2,
		},
		{
			name: "window fails", workspace: "$3", launchError: commandError, newWindow: true,
			wantMessage: "start codex in tmux window", wantCommands: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			commands := 0
			launcher := Launcher{
				clientName: "fixture",
				lookPath:   func(string) (string, error) { return "/tools/codex", nil },
				runCommand: func(_ context.Context, command string, _ ...string) ([]byte, error) {
					commands++
					if command == "display-message" {
						return []byte(test.workspace), test.lookupError
					}
					return nil, test.launchError
				},
			}

			err := launcher.Launch(context.Background(), "codex", t.TempDir(), test.newWindow)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("launch error = %v, want %q", err, test.wantMessage)
			}
			if test.lookupError != nil || test.launchError != nil {
				if !errors.Is(err, commandError) {
					t.Fatalf("launch error did not preserve command error: %v", err)
				}
			}
			if commands != test.wantCommands {
				t.Fatalf("ran %d commands, want %d", commands, test.wantCommands)
			}
		})
	}
}
