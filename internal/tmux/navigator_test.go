package tmux

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"a-gent/internal/agent"
)

func TestNavigatorOpensTheUniqueMatchingPane(t *testing.T) {
	var commands [][]string
	navigator := Navigator{
		clientName: "client-1",
		runCommand: func(_ context.Context, command string, arguments ...string) ([]byte, error) {
			commands = append(commands, append([]string{command}, arguments...))
			if command == "list-panes" {
				return []byte("%1\t/projects/other\tnvim\n%2\t/projects/a-gent\tcodex\n"), nil
			}
			return nil, nil
		},
	}

	err := navigator.Navigate(context.Background(), agent.Session{Provider: "codex", WorkingDirectory: "/projects/a-gent"})
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}

	wantCommands := [][]string{
		{"list-panes", "-a", "-F", "#{pane_id}\t#{pane_current_path}\t#{pane_start_command}"},
		{"switch-client", "-c", "client-1", "-t", "%2"},
	}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", commands, wantCommands)
	}
}

func TestNavigatorDoesNotGuessBetweenMatchingPanes(t *testing.T) {
	var switchAttempted bool
	navigator := Navigator{
		clientName: "client-1",
		runCommand: func(_ context.Context, command string, arguments ...string) ([]byte, error) {
			if command == "list-panes" {
				return []byte("%1\t/projects/a-gent\tcodex\n%2\t/projects/a-gent\tcodex\n"), nil
			}
			switchAttempted = true
			return nil, nil
		},
	}

	err := navigator.Navigate(context.Background(), agent.Session{Provider: "codex", WorkingDirectory: "/projects/a-gent"})
	if err == nil {
		t.Fatal("Navigate() error = nil, want an ambiguous-pane error")
	}
	if switchAttempted {
		t.Fatal("Navigate() attempted to switch panes despite an ambiguous match")
	}
}

func TestNavigatorReturnsTheTmuxSwitchError(t *testing.T) {
	switchError := errors.New("no such client")
	navigator := Navigator{
		clientName: "client-1",
		runCommand: func(_ context.Context, command string, arguments ...string) ([]byte, error) {
			if command == "list-panes" {
				return []byte("%1\t/projects/a-gent\tcodex\n"), nil
			}
			return nil, switchError
		},
	}

	err := navigator.Navigate(context.Background(), agent.Session{Provider: "codex", WorkingDirectory: "/projects/a-gent"})
	if !errors.Is(err, switchError) {
		t.Fatalf("Navigate() error = %v, want %v", err, switchError)
	}
}

func TestPaneRunsProviderChecksTheExecutable(t *testing.T) {
	testCases := []struct {
		command  string
		expected bool
	}{
		{command: "codex", expected: true},
		{command: "/usr/local/bin/codex --resume", expected: true},
		{command: "nvim codex"},
		{command: "echo codex"},
		{command: "codex-other"},
		{command: ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.command, func(t *testing.T) {
			if actual := paneRunsProvider(pane{command: testCase.command}, "codex"); actual != testCase.expected {
				t.Fatalf("paneRunsProvider() = %v, want %v", actual, testCase.expected)
			}
		})
	}
}

func TestNavigatorRejectsMissingTargets(t *testing.T) {
	for _, directory := range []string{"", "/projects/missing"} {
		t.Run(directory, func(t *testing.T) {
			navigator := Navigator{
				clientName: "client-1",
				runCommand: func(_ context.Context, command string, arguments ...string) ([]byte, error) {
					if command != "list-panes" {
						t.Fatalf("unexpected navigation command: %s", command)
					}
					return []byte("malformed\n%1\t/projects/missing\tnvim codex\n"), nil
				},
			}
			if err := navigator.Navigate(context.Background(), agent.Session{Provider: "codex", WorkingDirectory: directory}); err == nil {
				t.Fatal("missing target did not return an error")
			}
		})
	}
}

func TestNewNavigatorRequiresTmuxAndOriginatingClient(t *testing.T) {
	testCases := []struct {
		tmuxEnvironment, client string
		supported               bool
	}{
		{tmuxEnvironment: "", client: "client-1"},
		{tmuxEnvironment: "tmux", client: ""},
		{tmuxEnvironment: "tmux", client: "client-1", supported: true},
	}
	for _, testCase := range testCases {
		t.Setenv("TMUX", testCase.tmuxEnvironment)
		t.Setenv(tmuxClientEnvironmentVariable, testCase.client)
		if actual := NewNavigator() != nil; actual != testCase.supported {
			t.Fatalf("navigation supported = %v, want %v", actual, testCase.supported)
		}
	}
}
