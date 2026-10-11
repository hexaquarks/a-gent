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

	"a-gent/internal/hermes"
)

func TestIntegrationInstalledHermesLaunchAndNavigation(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_HERMES_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_HERMES_TESTS=1 to launch installed Hermes")
	}
	executable := os.Getenv("A_GENT_HERMES_EXECUTABLE")
	if executable == "" {
		var err error
		executable, err = exec.LookPath("hermes")
		if err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.MkdirTemp("/tmp", "ag-hermes-launch-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("HERMES_HOME", filepath.Join(root, "config"))
	t.Setenv("OPENAI_API_KEY", "local-fixture")
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.yaml"), []byte(`_config_version: 12
model:
  default: fixture-model
  provider: custom
  base_url: http://127.0.0.1:9/v1
  api_key: local-fixture
  context_length: 131072
display:
  interface: cli
telemetry:
  shared_metrics:
    enabled: false
    send: false
    offer_version: 2
`), 0600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "tmux")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run := func(ctx context.Context, command string, args ...string) ([]byte, error) {
		arguments := append([]string{"-S", socket, "-f", "/dev/null", command}, args...)
		return exec.CommandContext(ctx, "tmux", arguments...).CombinedOutput()
	}
	if output, err := run(ctx, "new-session", "-d", "-s", "launcher", "-x", "120", "-y", "32"); err != nil {
		t.Fatalf("start tmux: %v: %s", err, output)
	}
	defer stopLauncherFixture(t, socket)
	launcher := Launcher{clientName: "fixture", lookPath: func(string) (string, error) { return executable, nil },
		runCommand: func(ctx context.Context, command string, args ...string) ([]byte, error) {
			if command == "display-message" {
				args = []string{"-t", "launcher", "-p", args[len(args)-1]}
			}
			return run(ctx, command, args...)
		}}
	for range 2 {
		if err := launcher.Launch(ctx, "hermes", project, true); err != nil {
			t.Fatal(err)
		}
	}
	adapter := hermes.NewAdapter()
	var targets []string
	for ctx.Err() == nil {
		sessions, err := adapter.Sessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		targets = nil
		for _, session := range sessions {
			if session.WorkingDirectory != project || session.TranscriptID == "" {
				continue
			}
			var target string
			navigator := Navigator{clientName: "fixture", processParents: readProcessParents,
				runCommand: func(ctx context.Context, command string, args ...string) ([]byte, error) {
					if command == "switch-client" {
						target = args[len(args)-1]
						return nil, nil
					}
					return run(ctx, command, args...)
				}}
			if err := navigator.Navigate(ctx, session); err != nil {
				t.Fatal(err)
			}
			targets = append(targets, target)
		}
		if len(targets) == 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(targets) != 2 || targets[0] == targets[1] {
		t.Fatalf("same-project Hermes navigation targets: %v", targets)
	}
	output, err := run(ctx, "display-message", "-p", "-t", "launcher", "#{session_windows}")
	if err != nil || strings.TrimSpace(string(output)) != "3" {
		t.Fatalf("Hermes did not open new windows: %s, %v", output, err)
	}
	t.Log("Two installed Hermes instances opened in new windows and navigated to distinct panes in the same project")
}
