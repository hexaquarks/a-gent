package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"a-gent/internal/agent"
)

const tmuxClientEnvironmentVariable = "A_GENT_TMUX_CLIENT"

type commandRunner func(context.Context, string, ...string) ([]byte, error)

// Navigator opens tmux panes that match a coding-agent session's directory.
type Navigator struct {
	clientName string
	runCommand commandRunner
}

type pane struct {
	id        string
	directory string
	command   string
}

// NewNavigator returns nil outside a tmux popup or when its originating tmux
// client cannot be identified.
func NewNavigator() *Navigator {
	clientName := os.Getenv(tmuxClientEnvironmentVariable)
	if os.Getenv("TMUX") == "" || clientName == "" {
		return nil
	}

	return &Navigator{
		clientName: clientName,
		runCommand: commandRunner(runTmuxCommand),
	}
}

// Navigate opens the unique tmux agent pane that matches session's provider and directory.
func (navigator *Navigator) Navigate(context context.Context, session agent.Session) error {
	if session.WorkingDirectory == "" {
		return fmt.Errorf("session has no working directory")
	}

	panes, err := navigator.panes(context)
	if err != nil {
		return err
	}

	matchingPanes := matchingPanes(panes, session)
	if len(matchingPanes) == 0 {
		return fmt.Errorf("no tmux %s pane found for %s", session.Provider, session.WorkingDirectory)
	}
	if len(matchingPanes) > 1 {
		return fmt.Errorf("multiple tmux panes found for %s", session.WorkingDirectory)
	}

	if _, err := navigator.runCommand(context, "switch-client", "-c", navigator.clientName, "-t", matchingPanes[0].id); err != nil {
		return fmt.Errorf("open tmux pane: %w", err)
	}

	return nil
}

func (navigator *Navigator) panes(context context.Context) ([]pane, error) {
	output, err := navigator.runCommand(context, "list-panes", "-a", "-F", "#{pane_id}\t#{pane_current_path}\t#{pane_start_command}")
	if err != nil {
		return nil, fmt.Errorf("list tmux panes: %w", err)
	}

	var panes []pane
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		values := strings.SplitN(line, "\t", 3)
		if len(values) != 3 || values[0] == "" || values[1] == "" {
			continue
		}
		panes = append(panes, pane{id: values[0], directory: values[1], command: values[2]})
	}

	return panes, nil
}

func matchingPanes(panes []pane, session agent.Session) []pane {
	cleanDirectory := filepath.Clean(session.WorkingDirectory)
	matchingPanes := make([]pane, 0, 1)
	for _, pane := range panes {
		if filepath.Clean(pane.directory) == cleanDirectory && paneRunsProvider(pane, session.Provider) {
			matchingPanes = append(matchingPanes, pane)
		}
	}

	return matchingPanes
}

func paneRunsProvider(pane pane, provider string) bool {
	commandParts := strings.Fields(pane.command)
	if len(commandParts) == 0 || provider == "" {
		return false
	}

	// Only the executable identifies the provider. An argument such as
	// "nvim codex" must not make an editor pane look like an agent pane.
	return filepath.Base(commandParts[0]) == provider
}

func runTmuxCommand(context context.Context, command string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(context, "tmux", append([]string{command}, arguments...)...).Output()
}
