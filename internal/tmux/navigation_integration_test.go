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

	// One pane represents the requested provider; the other has the same directory.
	// Use the real ps output and tmux pane metadata, but intercept the client switch.
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
	if err := navigator.Navigate(context.Background(), session); err == nil || switchedTo != "" {
		t.Fatalf("ambiguous panes: target %q, error %v", switchedTo, err)
	}

	panes, err := run(context.Background(), "list-panes", "-t", "navigation:0", "-F", "#{pane_id}")
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(string(panes))
	if len(ids) != 2 {
		t.Fatalf("pane IDs = %q", panes)
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
}
