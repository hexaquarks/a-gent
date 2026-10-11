//go:build integration

package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestIntegrationPlainOpenCode(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_OPENCODE_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_OPENCODE_TESTS=1")
	}
	executable, err := exec.LookPath("opencode")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	project, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(root, "config", "opencode"))
	t.Setenv("OPENCODE_SERVER_PASSWORD", "")
	for _, key := range []string{
		"OPENCODE_DISABLE_AUTOUPDATE", "OPENCODE_DISABLE_PROJECT_CONFIG",
		"OPENCODE_DISABLE_MODELS_FETCH", "OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER",
	} {
		t.Setenv(key, "1")
	}
	if err := ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for i, delta := range []map[string]any{
			{"role": "assistant", "content": "Plain OpenCode completed"}, {},
		} {
			var finish any
			if i == 1 {
				finish = "stop"
			}
			data, _ := json.Marshal(map[string]any{
				"id": "fixture", "object": "chat.completion.chunk", "model": "deterministic",
				"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		close(release)
		model.Close()
	})
	config, _ := json.Marshal(map[string]any{
		"model": "fixture/deterministic", "small_model": "fixture/deterministic",
		"agent": map[string]any{"title": map[string]bool{"disable": true}},
		"provider": map[string]any{"fixture": map[string]any{
			"npm": "@ai-sdk/openai-compatible", "name": "Fixture",
			"options": map[string]string{"baseURL": model.URL + "/v1", "apiKey": "local-fixture"},
			"models": map[string]any{"deterministic": map[string]any{
				"name": "Deterministic", "limit": map[string]int{"context": 8192, "output": 1024},
			}},
		}},
	})
	t.Setenv("OPENCODE_CONFIG_CONTENT", string(config))
	socket := filepath.Join(root, "tmux.sock")
	tmux := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("tmux", append([]string{"-S", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux: %v: %s", err, output)
		}
		return string(output)
	}
	logPath := filepath.Join(root, "opencode.log")
	// Launch the real TUI without any HTTP-server flags.
	command := fmt.Sprintf("exec %s --prompt %s >%s 2>&1",
		quoteShell(executable), quoteShell("Reply with the fixture marker."), quoteShell(logPath))
	tmux("new-session", "-d", "-s", "fixture", "-c", project, command)
	pid := strings.TrimSpace(tmux("display-message", "-p", "-t", "fixture:0.0", "#{pane_pid}"))
	t.Cleanup(func() {
		exec.Command("kill", "-KILL", pid).Run()
		exec.Command("tmux", "-S", socket, "kill-server").Run()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("OpenCode log: %s", data)
		}
	})
	adapter := NewAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	adapter.discover = func(ctx context.Context) ([]instance, error) {
		instances, err := discoverInstances(ctx)
		if err != nil {
			return nil, err
		}
		var found []instance
		for _, instance := range instances {
			if instance.directory == project {
				found = append(found, instance)
			}
		}
		return found, nil
	}
	wait := func(description string, ready func() bool) {
		t.Helper()
		for ctx.Err() == nil {
			if ready() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("waiting for %s: %v", description, ctx.Err())
	}
	var selected agent.Session
	wait("plain TUI native running status", func() bool {
		sessions, err := adapter.Sessions(ctx)
		if err != nil {
			return false
		}
		for _, session := range sessions {
			if session.State == agent.StateRunning {
				selected = session
				return true
			}
		}
		return false
	})
	// The bridge must not expose operations that could change a user's session.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		selected.TranscriptPath+"/session?directory="+project, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("write permitted: %d", response.StatusCode)
	}
	release <- struct{}{}
	wait("idle and public preview", func() bool {
		sessions, err := adapter.Sessions(ctx)
		if err != nil {
			return false
		}
		for _, session := range sessions {
			if session.ID == selected.ID && session.State == agent.StateIdle {
				preview, err := adapter.LatestPreview(ctx, session)
				return err == nil && preview.Activity != nil &&
					strings.Contains(preview.Activity.Text, "Plain OpenCode completed")
			}
		}
		return false
	})
}

func quoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
