package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type process struct {
	parentID int
	command  string
}

// readProcesses includes commands so shell-launched agents can be found in a pane's process tree.
func readProcesses(ctx context.Context) (map[int]process, error) {
	output, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("read session processes: %w", err)
	}

	processes := make(map[int]process)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parentID, parentErr := strconv.Atoi(fields[1])
		if pidErr == nil && parentErr == nil {
			processes[pid] = process{parentID: parentID, command: strings.Join(fields[2:], " ")}
		}
	}

	return processes, nil
}

func paneHasProviderProcess(pane pane, processes map[int]process, provider string) bool {
	for pid, candidate := range processes {
		if !processRunsProvider(candidate.command, provider) {
			continue
		}
		seen := make(map[int]bool)
		for pid > 1 && !seen[pid] {
			if pid == pane.processID {
				return true
			}
			seen[pid] = true
			pid = processes[pid].parentID
		}
	}
	return false
}

func processRunsProvider(command, provider string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || provider == "" {
		return false
	}
	if filepath.Base(fields[0]) == provider {
		return true
	}
	// The npm launcher runs the provider's executable through node.
	return len(fields) > 1 && filepath.Base(fields[0]) == "node" && filepath.Base(fields[1]) == provider
}

// Records each process's parent so we can find the tmux pane containing an agent
// started inside a shell.
func readProcessParents(ctx context.Context) (map[int]int, error) {
	output, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil, fmt.Errorf("read session processes: %w", err)
	}

	parents := make(map[int]int)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parentProcessID, parentErr := strconv.Atoi(fields[1])
		if pidErr == nil && parentErr == nil {
			parents[pid] = parentProcessID
		}
	}

	return parents, nil
}

// readProcessTTY identifies the terminal of a live process even if it was reparented.
func readProcessTTY(ctx context.Context, processID int) (string, error) {
	output, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(processID), "-o", "tty=").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// A missing or detached terminal cannot identify a pane safely.
func panesForTerminal(panes []pane, terminal string) []pane {
	if terminal == "" || terminal == "??" || terminal == "?" {
		return nil
	}

	terminal = strings.TrimPrefix(terminal, "/dev/")
	var matches []pane
	for _, pane := range panes {
		if strings.TrimPrefix(pane.terminal, "/dev/") == terminal {
			matches = append(matches, pane)
		}
	}
	return matches
}

// Finds the pane by following the agent's parent processes back to the process
// tmux started. Stops if a process repeats so an invalid chain cannot loop forever.
func panesForProcess(panes []pane, parents map[int]int, processID int) []pane {
	ancestors := make(map[int]bool)
	for processID > 1 && !ancestors[processID] {
		// A session whose process has exited must not open another agent's pane.
		parent, alive := parents[processID]
		if !alive {
			break
		}
		ancestors[processID] = true
		processID = parent
	}

	var matches []pane
	for _, pane := range panes {
		if ancestors[pane.processID] {
			matches = append(matches, pane)
		}
	}

	return matches
}
