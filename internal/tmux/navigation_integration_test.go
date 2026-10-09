//go:build integration

package tmux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestIntegrationNavigationFindsAgentInsideShellPane(t *testing.T) {
	socketDirectory, err := os.MkdirTemp("/tmp", "ag-nav-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDirectory)
	socket := filepath.Join(socketDirectory, "tmux")
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "tmux", append([]string{"-S", socket}, args...)...).CombinedOutput()
	}
	defer run(context.Background(), "kill-server")

	directory := t.TempDir()
	if output, err := run(context.Background(), "new-session", "-d", "-s", "navigation", "-c", directory,
		"sh -c 'sleep 30 & wait'"); err != nil {
		t.Fatalf("start agent pane: %v: %s", err, output)
	}
	if output, err := run(context.Background(), "split-window", "-d", "-t", "navigation:0", "-c", directory,
		"sh -c 'sleep 30 & wait'"); err != nil {
		t.Fatalf("start second pane: %v: %s", err, output)
	}

	// Use real process and pane metadata while intercepting the client switch.
	var switchedTo string
	navigator := Navigator{
		clientName: "test",
		processes:  readProcesses,
		runCommand: func(ctx context.Context, command string, args ...string) ([]byte, error) {
			if command == "switch-client" {
				switchedTo = args[len(args)-1]
				return nil, nil
			}
			return run(ctx, append([]string{command}, args...)...)
		},
	}

	// Both panes initially contain sleep, so navigation must reject ambiguity.
	session := agent.Session{Provider: "sleep", WorkingDirectory: directory}
	readyBy := time.Now().Add(4 * time.Second)
	for {
		listedPanes, paneErr := navigator.panes(context.Background())
		processes, processErr := readProcesses(context.Background())
		if paneErr == nil && processErr == nil && len(matchingPanes(listedPanes, processes, session)) == 2 {
			break
		}
		if time.Now().After(readyBy) {
			t.Fatalf("agent panes did not start: panes %v, pane error %v, process error %v",
				listedPanes, paneErr, processErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
	panes, err := run(context.Background(), "list-panes", "-t", "navigation:0", "-F", "#{pane_id}")
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(string(panes))
	if len(ids) != 2 {
		t.Fatalf("pane IDs = %q", panes)
	}
	listedPanes, err := navigator.panes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	processes, err := readProcesses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	navigator.processParents = readProcessParents
	for _, id := range []string{ids[0], ids[1]} {
		processID := processInPane(listedPanes, processes, id, "sleep")
		if processID == 0 {
			t.Fatalf("could not find the agent process in pane %s", id)
		}
		if err := navigator.Navigate(context.Background(), agent.Session{
			Provider: "claude", WorkingDirectory: directory, ProcessID: &processID,
		}); err != nil || switchedTo != id {
			t.Fatalf("PID target = %q, want %q; error = %v", switchedTo, id, err)
		}
	}
	switchedTo = ""
	if err := navigator.Navigate(context.Background(), session); err == nil || switchedTo != "" {
		t.Fatalf("ambiguous panes: target %q, error %v", switchedTo, err)
	}

	if output, err := run(context.Background(), "kill-pane", "-t", ids[1]); err != nil {
		t.Fatalf("remove second pane: %v: %s", err, output)
	}
	if output, err := run(context.Background(), "split-window", "-d", "-t", "navigation:0", "-c", directory,
		"tail -f /dev/null"); err != nil {
		t.Fatalf("start unrelated pane: %v: %s", err, output)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		err = navigator.Navigate(context.Background(), session)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil || switchedTo != ids[0] {
		t.Fatalf("target = %q, want %q; error = %v", switchedTo, ids[0], err)
	}

	// Claude supplies a PID. If its parent chain is unavailable, the live PID's
	// terminal still identifies the exact pane among other panes in the project.
	listedPanes, err = navigator.panes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	processes, err = readProcesses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	processID := processInPane(listedPanes, processes, ids[0], "sleep")
	if processID == 0 {
		t.Fatal("could not find the agent process in the first pane")
	}

	navigator.processParents = func(context.Context) (map[int]int, error) { return map[int]int{}, nil }
	navigator.processTTY = readProcessTTY
	switchedTo = ""
	if err := navigator.Navigate(context.Background(), agent.Session{
		Provider: "claude", WorkingDirectory: directory, ProcessID: &processID,
	}); err != nil || switchedTo != ids[0] {
		t.Fatalf("Claude target = %q, want %q; error = %v", switchedTo, ids[0], err)
	}
}

func processInPane(panes []pane, processes map[int]process, paneID, provider string) int {
	var paneProcessID int
	for _, pane := range panes {
		if pane.id == paneID {
			paneProcessID = pane.processID
			break
		}
	}
	if paneProcessID == 0 {
		return 0
	}

	for pid, candidate := range processes {
		if !processRunsProvider(candidate.command, provider) {
			continue
		}
		seen := make(map[int]bool)
		for ancestor := pid; ancestor > 1 && !seen[ancestor]; ancestor = processes[ancestor].parentID {
			if ancestor == paneProcessID {
				return pid
			}
			seen[ancestor] = true
		}
	}
	return 0
}
