package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Launcher starts coding agents in the originating tmux client's workspace.
type Launcher struct {
	clientName string
	runCommand commandRunner
	lookPath   func(string) (string, error)
}

// NewLauncher returns nil when there is no tmux client to open a workspace in.
func NewLauncher() *Launcher {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	client := os.Getenv(tmuxClientEnvironmentVariable)
	if client == "" {
		client = activeClientName()
	}
	if client == "" {
		return nil
	}
	return &Launcher{clientName: client, runCommand: runTmuxCommand, lookPath: exec.LookPath}
}

// Launch opens an interactive provider CLI in directory, in a new pane or window.
func (launcher *Launcher) Launch(ctx context.Context, provider, directory string, newWindow bool) error {
	if provider != "codex" && provider != "claude" {
		return fmt.Errorf("unsupported provider: %s", provider)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project path is not a directory")
	}
	executable, err := launcher.lookPath(provider)
	if err != nil {
		return fmt.Errorf("%s is not installed or is not on PATH", provider)
	}
	// Resolve the parent client's workspace explicitly: the dashboard may run in a popup.
	command, targetFormat, destination := "split-window", "#{pane_id}", "pane"
	if newWindow {
		command, targetFormat, destination = "new-window", "#{session_id}", "window"
	}
	output, err := launcher.runCommand(ctx, "display-message", "-c", launcher.clientName, "-p", targetFormat)
	if err != nil {
		return fmt.Errorf("find tmux workspace: %w", err)
	}
	target := strings.TrimSpace(string(output))
	if target == "" {
		return fmt.Errorf("no tmux workspace found")
	}
	if newWindow {
		// An omitted window index lets tmux choose a free slot in the parent session.
		target += ":"
	}
	if _, err := launcher.runCommand(ctx, command, "-t", target, "-c", directory, shellQuote(executable)); err != nil {
		return fmt.Errorf("start %s in tmux %s: %w", provider, destination, err)
	}
	return nil
}
