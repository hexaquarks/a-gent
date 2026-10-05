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
				return []byte("%1\t/projects/other\tnvim\t0\n%2\t/projects/a-gent\tcodex\t0\n"), nil
			}
			return nil, nil
		},
	}

	err := navigator.Navigate(context.Background(), agent.Session{Provider: "codex", WorkingDirectory: "/projects/a-gent"})
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}

	wantCommands := [][]string{
		{"list-panes", "-a", "-F", "#{pane_id}\t#{pane_current_path}\t#{pane_start_command}\t#{pane_pid}"},
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
				return []byte("%1\t/projects/a-gent\tcodex\t0\n%2\t/projects/a-gent\tcodex\t0\n"), nil
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
				return []byte("%1\t/projects/a-gent\tcodex\t0\n"), nil
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
					return []byte("malformed\n%1\t/projects/missing\tnvim codex\t0\n"), nil
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

func TestNavigatorUsesProcessAncestryForShellLaunchedSessions(t *testing.T) {
	var target string
	navigator := Navigator{
		clientName:     "client",
		processParents: func(context.Context) (map[int]int, error) { return map[int]int{42: 20, 20: 10, 10: 1}, nil },
		runCommand: func(_ context.Context, command string, arguments ...string) ([]byte, error) {
			if command == "list-panes" {
				return []byte("%1\t/project\t\t10\n%2\t/project\tclaude\t30\n"), nil
			}
			target = arguments[len(arguments)-1]
			return nil, nil
		},
	}
	// Both panes share the project; process identity must select the shell pane.
	processID := 42
	err := navigator.Navigate(context.Background(), agent.Session{Provider: "claude", WorkingDirectory: "/project", ProcessID: &processID})
	if err != nil || target != "%1" {
		t.Fatalf("target = %q, error = %v", target, err)
	}
}

func TestNavigatorDoesNotFallBackAfterProcessExit(t *testing.T) {
	navigator := Navigator{
		processParents: func(context.Context) (map[int]int, error) { return map[int]int{}, nil },
		runCommand: func(_ context.Context, command string, _ ...string) ([]byte, error) {
			if command != "list-panes" {
				t.Fatal("switched to an unrelated session")
			}
			return []byte("%1\t/project\tclaude\t10\n"), nil
		},
	}
	processID := 42
	if err := navigator.Navigate(context.Background(), agent.Session{Provider: "claude", WorkingDirectory: "/project", ProcessID: &processID}); err == nil {
		t.Fatal("matched an exited session")
	}
}

func TestProcessAncestryHandlesCyclesAndDirectProcesses(t *testing.T) {
	panes := []pane{{id: "direct", processID: 42}, {id: "unrelated", processID: 50}}
	matches := panesForProcess(panes, map[int]int{42: 43, 43: 42}, 42)
	if len(matches) != 1 || matches[0].id != "direct" {
		t.Fatalf("matches = %+v", matches)
	}
}

func TestNavigatorRejectsKnownProcesslessSessions(t *testing.T) {
	processID := 0
	navigator := Navigator{
		runCommand: func(_ context.Context, command string, _ ...string) ([]byte, error) {
			if command != "list-panes" {
				t.Fatal("attempted to navigate a processless session")
			}
			return []byte("%1\t/project\tclaude\t10\n"), nil
		},
	}
	session := agent.Session{Provider: "claude", WorkingDirectory: "/project", ProcessID: &processID}
	if err := navigator.Navigate(context.Background(), session); err == nil {
		t.Fatal("matched an unrelated Claude pane by directory")
	}
}
