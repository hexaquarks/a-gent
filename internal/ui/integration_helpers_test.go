//go:build integration

package ui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/claude"
	"a-gent/internal/codex"
	"a-gent/internal/polling"
	"a-gent/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

const fixtureSessionID = "integration-session"

// Discovery is deterministic; activity and edits use the production adapters.
// This keeps the integration suite independent of installed agents and accounts.
type fixtureProvider struct {
	name         string
	metadataPath string
	source       agent.PreviewSource
}

func (provider fixtureProvider) Provider() string { return provider.name }
func (provider fixtureProvider) Sessions(ctx context.Context) ([]agent.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(provider.metadataPath)
	if err != nil {
		return nil, err
	}
	var sessions []agent.Session
	err = json.Unmarshal(data, &sessions)
	return sessions, err
}
func (provider fixtureProvider) LatestPreview(ctx context.Context, session agent.Session) (agent.Preview, error) {
	return provider.source.LatestPreview(ctx, session)
}

// TestIntegrationDashboardProcess is invoked only as a child test process in a
// real terminal. The parent tests drive its input and inspect the rendered screen.
func TestIntegrationDashboardProcess(t *testing.T) {
	root := os.Getenv("A_GENT_TEST_FIXTURE")
	if root == "" {
		t.Skip("helper process for terminal integration tests")
	}
	adapters := []agent.Adapter{
		fixtureProvider{"codex", filepath.Join(root, "codex-sessions.json"), codex.NewAdapter()},
		fixtureProvider{"claude", filepath.Join(root, "claude-sessions.json"), claude.NewAdapter()},
	}
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan polling.Update)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		polling.Run(ctx, adapters, updates)
	}()
	defer func() { cancel(); <-stopped }()
	program := tea.NewProgram(ui.NewModel(updates, ui.WithApplicationContext(ctx), ui.WithPreviewSources(adapters...)), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		t.Fatal(err)
	}
}

type dashboardFixture struct {
	t             *testing.T
	root          string
	socket        string
	artifacts     string
	transcripts   map[string]string
	sessions      map[string]agent.Session
	width, height int
}

func newDashboardFixture(t *testing.T) *dashboardFixture {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal("integration tests require tmux on PATH")
	}
	// macOS Unix socket paths have a short limit, so keep the socket out of TempDir.
	socketDirectory, err := os.MkdirTemp("/tmp", "ag-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDirectory) })
	fixture := &dashboardFixture{t: t, root: t.TempDir(), socket: filepath.Join(socketDirectory, "tmux"), width: ui.PopupWidth, height: ui.PopupContentHeight}
	fixture.artifacts = filepath.Join(fixture.root, "screens")
	if destination := os.Getenv("A_GENT_TEST_ARTIFACTS"); destination != "" {
		fixture.artifacts = filepath.Join(destination, strings.ReplaceAll(t.Name(), "/", "-"))
	}
	fixture.transcripts = map[string]string{
		"codex":  filepath.Join(fixture.root, "codex.jsonl"),
		"claude": filepath.Join(fixture.root, "claude", "projects", "fixture", fixtureSessionID+".jsonl"),
	}
	fixture.sessions = make(map[string]agent.Session)
	for _, provider := range []string{"codex", "claude"} {
		path := fixture.transcripts[provider]
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		session := agent.Session{ID: fixtureSessionID, TranscriptID: fixtureSessionID, TranscriptPath: path, Provider: provider, Name: provider + " fixture", WorkingDirectory: "/fixture/" + provider, State: agent.StateRunning}
		// Keep row order stable across independently arriving provider polls.
		if provider == "codex" {
			session.LastActiveAt = time.Now()
		}
		fixture.sessions[provider] = session
		fixture.setState(provider, agent.StateRunning)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := []string{"A_GENT_TEST_FIXTURE=" + fixture.root, "CLAUDE_CONFIG_DIR=" + filepath.Join(fixture.root, "claude"), "TERM=xterm-256color", "COLORTERM=truecolor", "CLICOLOR_FORCE=1"}
	command := []string{"env", "-u", "NO_COLOR"}
	command = append(command, environment...)
	command = append(command, binary, "-test.run=^TestIntegrationDashboardProcess$", "-test.timeout=60s")
	for i, arg := range command {
		command[i] = shellQuote(arg)
	}
	t.Cleanup(func() {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, "tmux", "-S", fixture.socket, "kill-server").Run()
		}()
		if t.Failed() {
			fixture.capture("failure")
		} else {
			fixture.key("q")
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				state, err := fixture.tmuxOutput("display-message", "-p", "-t", "dashboard:0.0", "#{pane_dead}:#{pane_dead_status}")
				if err != nil {
					t.Errorf("inspect dashboard exit: %v", err)
					break
				}
				if strings.HasPrefix(state, "1:") {
					if strings.TrimSpace(state) != "1:0" {
						t.Errorf("dashboard failed on exit: %s", state)
						fixture.capture("exit-failure")
					}
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if time.Now().After(deadline) {
				t.Error("dashboard did not exit after q")
			}
		}
	})
	fixture.tmux("-f", "/dev/null", "new-session", "-d", "-s", "dashboard", "-x", fmt.Sprint(fixture.width), "-y", fmt.Sprint(fixture.height), strings.Join(command, " "))
	fixture.tmux("set-option", "-g", "status", "off")
	fixture.tmux("set-window-option", "-g", "remain-on-exit", "on")
	fixture.tmux("set-window-option", "-g", "window-size", "manual")
	fixture.waitFor("both providers", 8*time.Second, func(screen string) bool { return strings.Contains(screen, "2 sessions") })
	fixture.selectProvider("codex")
	return fixture
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func (fixture *dashboardFixture) tmux(args ...string) string {
	fixture.t.Helper()
	output, err := fixture.tmuxOutput(args...)
	if err != nil {
		fixture.t.Fatalf("tmux %v: %v\n%s", args, err, output)
	}
	return output
}
func (fixture *dashboardFixture) tmuxOutput(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "tmux", append([]string{"-S", fixture.socket}, args...)...).CombinedOutput()
	return string(output), err
}
func (fixture *dashboardFixture) key(key string) {
	fixture.t.Helper()
	fixture.tmux("send-keys", "-t", "dashboard:0.0", key)
}

func (fixture *dashboardFixture) waitFor(description string, timeout time.Duration, ready func(string) bool) string {
	fixture.t.Helper()
	deadline := time.Now().Add(timeout)
	var screen string
	for time.Now().Before(deadline) {
		screen = fixture.tmux("capture-pane", "-p", "-t", "dashboard:0.0")
		if ready(screen) {
			return screen
		}
		time.Sleep(50 * time.Millisecond)
	}
	fixture.t.Fatalf("timed out after %s waiting for %s:\n%s", timeout, description, screen)
	return ""
}
func (fixture *dashboardFixture) waitText(text string) string {
	fixture.t.Helper()
	return fixture.waitFor(text, 4*time.Second, func(screen string) bool { return strings.Contains(screen, text) })
}
func (fixture *dashboardFixture) selectProvider(provider string) {
	fixture.t.Helper()
	key := "g"
	if provider == "claude" {
		key = "G"
	}
	fixture.key(key)
	fixture.waitText("/fixture/" + provider)
}
func (fixture *dashboardFixture) setState(provider string, state agent.State) {
	fixture.t.Helper()
	session := fixture.sessions[provider]
	session.State = state
	fixture.sessions[provider] = session
	data, err := json.Marshal([]agent.Session{session})
	if err != nil {
		fixture.t.Fatal(err)
	}
	path := filepath.Join(fixture.root, provider+"-sessions.json")
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		fixture.t.Fatal(err)
	}
}
func (fixture *dashboardFixture) appendRecord(provider string, record any) {
	fixture.t.Helper()
	file, err := os.OpenFile(fixture.transcripts[provider], os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fixture.t.Fatal(err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(record); err != nil {
		fixture.t.Fatal(err)
	}
}
func (fixture *dashboardFixture) appendOutput(provider, text string) {
	fixture.t.Helper()
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	if provider == "codex" {
		fixture.appendRecord(provider, map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{
			"type": "message", "role": "assistant", "phase": "commentary", "content": []any{map[string]string{"type": "output_text", "text": text}},
		}})
	} else {
		fixture.appendRecord(provider, map[string]any{"type": "assistant", "sessionId": fixtureSessionID, "timestamp": timestamp, "message": map[string]any{
			"content": []any{map[string]string{"type": "text", "text": text}},
		}})
	}
}
func (fixture *dashboardFixture) appendEdit(status, filename string) {
	fixture.t.Helper()
	fixture.appendRecord("codex", map[string]any{"type": "event_msg", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "payload": map[string]any{
		"type": "item_completed", "thread_id": fixtureSessionID, "item": map[string]any{
			"type": "FileChange", "status": status, "changes": map[string]any{filename: map[string]string{
				"type": "update", "unified_diff": "@@ -1,3 +1,3 @@\n context\n-old value\n+verified change\n context",
			}},
		},
	}})
}

// Capture plain and ANSI screens plus an asciicast for optional visual review.
// Images can be rendered from the cast with agg; image tools are not test dependencies.
func (fixture *dashboardFixture) capture(name string) {
	fixture.t.Helper()
	plain, err := fixture.tmuxOutput("capture-pane", "-p", "-t", "dashboard:0.0")
	if err != nil {
		fixture.t.Logf("capture unavailable: %v", err)
		return
	}
	colored, err := fixture.tmuxOutput("capture-pane", "-p", "-e", "-t", "dashboard:0.0")
	if err != nil {
		fixture.t.Logf("color capture unavailable: %v", err)
		return
	}
	if err := os.MkdirAll(fixture.artifacts, 0700); err != nil {
		fixture.t.Error(err)
		return
	}
	var frame strings.Builder
	frame.WriteString("\x1b[2J")
	for i, line := range strings.Split(strings.TrimSuffix(colored, "\n"), "\n") {
		fmt.Fprintf(&frame, "\x1b[%d;1H%s", i+1, line)
	}
	frame.WriteString("\x1b[?25l")
	var cast strings.Builder
	encoder := json.NewEncoder(&cast)
	_ = encoder.Encode(map[string]any{
		"version": 2, "width": fixture.width, "height": fixture.height,
		"theme": map[string]string{
			"fg": "#EDF2F4", "bg": "#0C1112",
			"palette": "#000000:#cd0000:#00cd00:#cdcd00:#0000ee:#cd00cd:#00cdcd:#e5e5e5:" +
				"#7f7f7f:#ff0000:#00ff00:#ffff00:#5c5cff:#ff00ff:#00ffff:#ffffff",
		},
	})
	_ = encoder.Encode([]any{0, "o", frame.String()})
	_ = encoder.Encode([]any{0.2, "o", ""})
	for extension, data := range map[string]string{"txt": plain, "ansi": colored, "cast": cast.String()} {
		if err := os.WriteFile(filepath.Join(fixture.artifacts, name+"."+extension), []byte(data), 0600); err != nil {
			fixture.t.Error(err)
		}
	}
	fixture.t.Logf("terminal capture: %s", filepath.Join(fixture.artifacts, name+".txt"))
}
