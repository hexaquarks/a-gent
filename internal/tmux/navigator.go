package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"a-gent/internal/agent"
)

const tmuxClientEnvironmentVariable = "A_GENT_TMUX_CLIENT"

type commandRunner func(context.Context, string, ...string) ([]byte, error)

// Navigator finds the tmux pane containing a coding-agent session.
type Navigator struct {
	clientName     string
	runCommand     commandRunner
	processParents func(context.Context) (map[int]int, error)
}

type pane struct {
	id        string
	directory string
	command   string
	processID int
}

// NewNavigator returns nil outside a tmux popup or when its originating tmux
// client cannot be identified.
func NewNavigator() *Navigator {
	clientName := os.Getenv(tmuxClientEnvironmentVariable)
	if os.Getenv("TMUX") == "" || clientName == "" {
		return nil
	}

	return &Navigator{
		clientName:     clientName,
		runCommand:     commandRunner(runTmuxCommand),
		processParents: readProcessParents,
	}
}

// Navigate opens the session's pane using its process ID when available.
// Otherwise, it requires a single pane running that provider in the same directory.
func (navigator *Navigator) Navigate(context context.Context, session agent.Session) error {
	if session.WorkingDirectory == "" && session.ProcessID == nil {
		return fmt.Errorf("session has no working directory")
	}

	panes, err := navigator.panes(context)
	if err != nil {
		return err
	}

	var targets []pane
	if session.ProcessID != nil {
		if *session.ProcessID <= 0 {
			return fmt.Errorf("session has no live local process")
		}
		parents, err := navigator.processParents(context)
		if err != nil {
			return err
		}
		targets = panesForProcess(panes, parents, *session.ProcessID)
	} else {
		targets = matchingPanes(panes, session)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no tmux %s pane found for %s", session.Provider, session.WorkingDirectory)
	}
	if len(targets) > 1 {
		return fmt.Errorf("multiple tmux panes found for %s", session.WorkingDirectory)
	}

	if _, err := navigator.runCommand(context, "switch-client", "-c", navigator.clientName, "-t", targets[0].id); err != nil {
		return fmt.Errorf("open tmux pane: %w", err)
	}

	return nil
}

func (navigator *Navigator) panes(context context.Context) ([]pane, error) {
	output, err := navigator.runCommand(context, "list-panes", "-a", "-F", "#{pane_id}\t#{pane_current_path}\t#{pane_start_command}\t#{pane_pid}")
	if err != nil {
		return nil, fmt.Errorf("list tmux panes: %w", err)
	}

	var panes []pane
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		values := strings.SplitN(line, "\t", 4)
		if len(values) != 4 || values[0] == "" || values[1] == "" {
			continue
		}
		processID, _ := strconv.Atoi(values[3])
		panes = append(panes, pane{id: values[0], directory: values[1], command: values[2], processID: processID})
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
