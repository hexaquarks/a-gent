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
)

func TestIntegrationLauncherStartsProviderInProject(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal("integration tests require tmux")
	}
	for _, newWindow := range []bool{false, true} {
		name := "pane"
		if newWindow {
			name = "window"
		}
		t.Run(name, func(t *testing.T) {
			// Keep the socket path short enough for macOS Unix sockets.
			root, err := os.MkdirTemp("/tmp", "ag-launch-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(root)
			socket := filepath.Join(root, "tmux")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			run := func(ctx context.Context, command string, args ...string) ([]byte, error) {
				return exec.CommandContext(ctx, "tmux", append([]string{"-S", socket, "-f", "/dev/null", command}, args...)...).CombinedOutput()
			}
			defer func() { exec.Command("tmux", "-S", socket, "kill-server").Run() }()
			if output, err := run(ctx, "new-session", "-d", "-s", "launcher", "-x", "120", "-y", "30"); err != nil {
				t.Fatalf("start tmux: %v: %s", err, output)
			}
			project := filepath.Join(root, "project's directory")
			if err := os.Mkdir(project, 0700); err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(root, "provider's executable")
			// A local stand-in records its working directory and waits for input.
			// No installed provider, credentials, or model calls are involved.
			if err := os.WriteFile(executable, []byte("#!/bin/sh\npwd > launched-directory\nexec cat\n"), 0700); err != nil {
				t.Fatal(err)
			}
			launcher := Launcher{
				clientName: "fixture-client",
				lookPath:   func(string) (string, error) { return executable, nil },
				runCommand: func(ctx context.Context, command string, args ...string) ([]byte, error) {
					if command == "display-message" {
						// The isolated server has no attached client; resolve its known workspace.
						args = []string{"-t", "launcher", "-p", args[len(args)-1]}
					}
					return run(ctx, command, args...)
				},
			}
			if err := launcher.Launch(ctx, "codex", project, newWindow); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(project, "launched-directory")
			for {
				data, err := os.ReadFile(marker)
				if err == nil && len(data) > 0 {
					expected, err := filepath.EvalSymlinks(project)
					if err != nil {
						t.Fatal(err)
					}
					actual, err := filepath.EvalSymlinks(strings.TrimSpace(string(data)))
					if err != nil {
						t.Fatal(err)
					}
					if actual != expected {
						t.Fatalf("provider started in %q, want %q", data, expected)
					}
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("provider did not start: %v", ctx.Err())
				case <-time.After(20 * time.Millisecond):
				}
			}
			format := "#{window_panes}"
			if newWindow {
				format = "#{session_windows}"
			}
			output, err := run(ctx, "display-message", "-p", "-t", "launcher", format)
			if err != nil || strings.TrimSpace(string(output)) != "2" {
				t.Fatalf("wrong tmux destination: %q, %v", output, err)
			}
		})
	}
}
