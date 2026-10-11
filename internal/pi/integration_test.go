//go:build integration

package pi

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestIntegrationInstalledPiNativeStatusAndPreview(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_PI_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_PI_TESTS=1 to test installed Pi")
	}
	executable := os.Getenv("A_GENT_PI_EXECUTABLE")
	if executable == "" {
		var err error
		executable, err = exec.LookPath("pi")
		if err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	project, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "fixture.txt"), []byte("old value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	configDirectory := filepath.Join(root, "config")
	t.Setenv("PI_CODING_AGENT_DIR", configDirectory)
	observerPath, err := ConfigureObserver()
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var requests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		sequence := requests.Add(1)
		if sequence == 1 {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var delta map[string]any
		finish := "stop"
		if sequence == 1 {
			finish = "tool_calls"
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": "fixture-edit", "type": "function", "function": map[string]any{
					"name":      "edit",
					"arguments": `{"path":"fixture.txt","edits":[{"oldText":"old value","newText":"verified change"}]}`,
				},
			}}}
		} else {
			delta = map[string]any{"role": "assistant", "content": "Installed Pi fixture completed"}
		}
		for i, part := range []map[string]any{delta, {}} {
			var reason any
			if i == 1 {
				reason = finish
			}
			chunk := map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1,
				"model": "deterministic", "choices": []any{map[string]any{"index": 0, "delta": part, "finish_reason": reason}}}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		unblock()
		model.Close()
	})
	configuration := map[string]any{"providers": map[string]any{"fixture": map[string]any{
		"baseUrl": model.URL + "/v1", "api": "openai-completions", "apiKey": "local-fixture",
		"models": []any{map[string]any{"id": "deterministic", "name": "Fixture", "reasoning": false,
			"input": []string{"text"}, "contextWindow": 8192, "maxTokens": 1024,
			"cost": map[string]int{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0}}},
	}}}
	data, _ := json.Marshal(configuration)
	if err := os.WriteFile(filepath.Join(configDirectory, "models.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--mode", "rpc",
		"--provider", "fixture", "--model", "deterministic", "--thinking", "off", "--tools", "edit")
	command.Dir = project
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(root, "pi.log"))
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
			data, _ := os.ReadFile(logFile.Name())
			t.Logf("Pi log:\n%s", data)
		}
	})
	adapter := NewAdapter()
	adapter.directory = filepath.Dir(observerPath)
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
	wait("native idle session", func() bool {
		sessions, err := adapter.Sessions(ctx)
		return err == nil && len(sessions) == 1 && sessions[0].State == agent.StateIdle
	})
	if _, err := fmt.Fprintln(input, `{"type":"prompt","message":"Edit fixture.txt and report completion."}`); err != nil {
		t.Fatal(err)
	}
	var running agent.Session
	wait("native busy state", func() bool {
		sessions, err := adapter.Sessions(ctx)
		if err == nil && len(sessions) == 1 && sessions[0].State == agent.StateRunning && requests.Load() > 0 {
			running = sessions[0]
			return true
		}
		return false
	})
	unblock()
	wait("native idle, public output and completed edit", func() bool {
		sessions, err := adapter.Sessions(ctx)
		if err != nil || len(sessions) != 1 || sessions[0].ID != running.ID || sessions[0].State != agent.StateIdle {
			return false
		}
		preview, err := adapter.LatestPreview(ctx, sessions[0])
		return err == nil && preview.Activity != nil && preview.Activity.Text == "Installed Pi fixture completed" &&
			preview.Edit != nil && preview.Edit.Filename == "fixture.txt" &&
			strings.Contains(preview.Edit.Diff, "verified change")
	})
	t.Log("Plain Pi launch loaded the user observer; native running → idle, public preview and applied edit verified")
}
