package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

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
