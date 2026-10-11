//go:build integration

package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"a-gent/internal/agent"
)

// Opt-in coverage verifies the installed CLI without credentials or a paid model.
// The local model fixture holds a response so the native busy state is observable.
func TestIntegrationInstalledOpenCode(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_OPENCODE_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_OPENCODE_TESTS=1 to test installed OpenCode")
	}
	executable, err := exec.LookPath("opencode")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
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
		chunks := []map[string]any{
			{"role": "assistant", "content": "Installed OpenCode fixture completed"},
			{},
		}
		for i, delta := range chunks {
			var finishReason any
			if i == len(chunks)-1 {
				finishReason = "stop"
			}
			chunk := map[string]any{
				"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "deterministic",
				"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finishReason}},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		unblock()
		model.Close()
	})
	configuration := map[string]any{
		"model":       "fixture/deterministic",
		"small_model": "fixture/deterministic",
		"agent":       map[string]any{"title": map[string]bool{"disable": true}},
		"provider": map[string]any{"fixture": map[string]any{
			"npm": "@ai-sdk/openai-compatible", "name": "Fixture",
			"options": map[string]string{"baseURL": model.URL + "/v1", "apiKey": "local-fixture"},
			"models": map[string]any{"deterministic": map[string]any{
				"name": "Deterministic", "limit": map[string]int{"context": 8192, "output": 1024},
			}},
		}},
	}
	config, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "serve", "--port", "0", "--hostname", "127.0.0.1")
	command.Dir = project
	command.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"), "XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"), "OPENCODE_CONFIG_DIR="+filepath.Join(root, "config", "opencode"),
		"OPENCODE_CONFIG_CONTENT="+string(config), "OPENCODE_SERVER_PASSWORD=", "OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_MODELS_FETCH=1", "OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER=1",
	)
	logPath := filepath.Join(root, "opencode.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		command.Process.Kill()
		command.Wait()
		logFile.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("OpenCode log:\n%s", data)
		}
	})
	adapter := NewAdapter()
	adapter.discover = func(ctx context.Context) ([]instance, error) {
		entries, err := discoverInstances(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.pid == command.Process.Pid {
				return []instance{entry}, nil
			}
		}
		return nil, nil
	}
	var endpoint string
	wait := func(description string, ready func() bool) {
		t.Helper()
		for ctx.Err() == nil {
			if ready() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("waiting for %s: %v", description, ctx.Err())
	}
	wait("local API", func() bool {
		entries, err := adapter.discover(ctx)
		if err != nil || len(entries) == 0 || entries[0].endpoint == "" {
			return false
		}
		endpoint = entries[0].endpoint
		sessions, err := adapter.Sessions(ctx)
		return err == nil && len(sessions) == 1 && sessions[0].State == agent.StateIdle
	})
	post := func(path string, payload any, target any) {
		t.Helper()
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost,
			endpoint+path+"?directory="+url.QueryEscape(project), bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := adapter.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("%s: HTTP %d", path, response.StatusCode)
		}
		if target != nil {
			if err := json.NewDecoder(response.Body).Decode(target); err != nil {
				t.Fatal(err)
			}
		}
	}
	var session sessionRecord
	post("/session", map[string]string{"title": "Installed OpenCode fixture"}, &session)
	post("/session/"+session.ID+"/prompt_async", map[string]any{
		"model": map[string]string{"providerID": "fixture", "modelID": "deterministic"},
		"parts": []any{map[string]string{"type": "text", "text": "Reply with the fixture marker."}},
	}, nil)
	var running agent.Session
	wait("native running state", func() bool {
		sessions, err := adapter.Sessions(ctx)
		if err != nil {
			return false
		}
		for _, entry := range sessions {
			if entry.TranscriptID == session.ID && entry.State == agent.StateRunning {
				running = entry
				return true
			}
		}
		return false
	})
	unblock()
	wait("native idle state and public output", func() bool {
		sessions, err := adapter.Sessions(ctx)
		if err != nil {
			return false
		}
		for _, entry := range sessions {
			if entry.ID != running.ID || entry.State != agent.StateIdle {
				continue
			}
			preview, err := adapter.LatestPreview(ctx, entry)
			return err == nil && preview.Activity != nil &&
				strings.Contains(preview.Activity.Text, "Installed OpenCode fixture completed")
		}
		return false
	})
	t.Log("Installed CLI discovery, native running → idle, and public preview verified")
}
