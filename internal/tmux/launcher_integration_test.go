//go:build integration

package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestIntegrationLauncherStartsProviderInProject(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal("integration tests require tmux")
	}
	for _, provider := range []string{"codex", "claude", "opencode", "pi", "hermes"} {
		for _, newWindow := range []bool{false, true} {
			name := "pane"
			if newWindow {
				name = "window"
			}
			t.Run(provider+"/"+name, func(t *testing.T) {
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
				defer stopLauncherFixture(t, socket)
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
					clientName:     "fixture-client",
					hermesObserver: func() (string, error) { return "observer.py", nil },
					piObserver:     func() (string, error) { return filepath.Join(root, "observer.ts"), nil },
					lookPath:       func(string) (string, error) { return executable, nil },
					runCommand: func(ctx context.Context, command string, args ...string) ([]byte, error) {
						if command == "display-message" {
							// The isolated server has no attached client; resolve its known workspace.
							args = []string{"-t", "launcher", "-p", args[len(args)-1]}
						}
						return run(ctx, command, args...)
					},
				}
				if err := launcher.Launch(ctx, provider, project, newWindow); err != nil {
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
}

func TestIntegrationInstalledOpenCodeLaunch(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_OPENCODE_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_OPENCODE_TESTS=1 to launch installed OpenCode")
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "ag-oc-launch-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	socket := filepath.Join(root, "tmux")
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run := func(ctx context.Context, command string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "tmux", append([]string{"-S", socket, "-f", "/dev/null", command}, args...)...)
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(root, "config"),
			"XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_CACHE_HOME="+filepath.Join(root, "cache"),
			"OPENCODE_CONFIG_DIR="+filepath.Join(root, "config", "opencode"), "OPENCODE_CONFIG_CONTENT={}",
			"OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_MODELS_FETCH=1", "OPENCODE_DISABLE_AUTOUPDATE=1",
			"OPENCODE_SERVER_PASSWORD=", "TERM=xterm-256color")
		return cmd.CombinedOutput()
	}
	defer stopLauncherFixture(t, socket)
	if output, err := run(ctx, "new-session", "-d", "-s", "launcher", "-x", "120", "-y", "32"); err != nil {
		t.Fatalf("start tmux: %v: %s", err, output)
	}
	launcher := Launcher{
		clientName: "fixture-client", lookPath: exec.LookPath,
		runCommand: func(ctx context.Context, command string, args ...string) ([]byte, error) {
			if command == "display-message" {
				args = []string{"-t", "launcher", "-p", args[len(args)-1]}
			}
			return run(ctx, command, args...)
		},
	}
	if err := launcher.Launch(ctx, "opencode", project, true); err != nil {
		t.Fatal(err)
	}
	var screen string
	for ctx.Err() == nil {
		output, err := run(ctx, "capture-pane", "-p", "-t", "launcher:1.0")
		screen = string(output)
		if err == nil && strings.Contains(screen, "Ask anything") {
			output, err := run(ctx, "display-message", "-p", "-t", "launcher:1.0", "#{pane_current_path}")
			actual, pathErr := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
			expected, _ := filepath.EvalSymlinks(project)
			if err != nil || pathErr != nil || actual != expected {
				t.Fatalf("wrong project: %q, %v", output, err)
			}
			t.Log("Installed OpenCode TUI opened in a new tmux window in the selected project")
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("OpenCode did not open: %v\n%s", ctx.Err(), screen)
}

// A provider can ignore the terminal hangup from kill-server. Kill the fixture's
// pane process groups first, while tmux can still identify and reap them.
func stopLauncherFixture(t *testing.T, socket string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "tmux", "-S", socket,
		"list-panes", "-a", "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Errorf("list fixture processes before cleanup: %v", err)
	}
	var groups []int
	for _, field := range strings.Fields(string(output)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 1 {
			t.Errorf("invalid fixture pane PID: %q", field)
			continue
		}
		group, err := syscall.Getpgid(pid)
		if errors.Is(err, syscall.ESRCH) {
			continue
		}
		// tmux creates each pane as a process group leader. Never signal an
		// unexpected group that could contain the test runner or other work.
		if err != nil || group != pid {
			t.Errorf("fixture pane %d has unexpected process group %d: %v", pid, group, err)
			continue
		}
		groups = append(groups, group)
		if err := syscall.Kill(-group, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			t.Errorf("kill fixture process group %d: %v", group, err)
		}
	}
	// Killing the final pane may already have stopped the server.
	exec.CommandContext(ctx, "tmux", "-S", socket, "kill-server").Run()
	for _, group := range groups {
		for !errors.Is(syscall.Kill(-group, 0), syscall.ESRCH) {
			if ctx.Err() != nil {
				t.Errorf("fixture process group %d survived cleanup", group)
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestIntegrationLauncherCleanupKillsHangupIgnoringProvider(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "ag-cleanup-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	socket := filepath.Join(root, "tmux")
	marker := filepath.Join(root, "ready")
	// Reproduce a provider that survives tmux's terminal hangup.
	command := "trap '' HUP; touch " + shellQuote(marker) + "; while :; do sleep 1; done"
	output, err := exec.Command("tmux", "-S", socket, "-f", "/dev/null",
		"new-session", "-d", "-s", "cleanup", "sh -c "+shellQuote(command)).CombinedOutput()
	if err != nil {
		t.Fatalf("start fixture: %v: %s", err, output)
	}
	defer stopLauncherFixture(t, socket)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hangup-ignoring provider did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
