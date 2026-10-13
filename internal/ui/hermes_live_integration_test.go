//go:build integration

package ui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"a-gent/internal/hermes"
)

type liveHermesFixture struct {
	release chan struct{}
	pid     int
	stop    sync.Once
}

func (fixture *dashboardFixture) stopLiveHermes() {
	if fixture.liveHermes == nil {
		return
	}
	fixture.liveHermes.stop.Do(func() {
		if fixture.t.Failed() {
			screen, _ := fixture.tmuxOutput("capture-pane", "-p", "-t", "hermes:0")
			fixture.t.Logf("Hermes terminal:\n%s", screen)
			data, _ := os.ReadFile(filepath.Join(fixture.root, "hermes-config", "logs", "agent.log"))
			fixture.t.Logf("Hermes log:\n%s", data)
		}
		process, err := os.FindProcess(fixture.liveHermes.pid)
		if err == nil {
			_ = process.Kill()
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if exec.Command("/bin/ps", "-p", strconv.Itoa(fixture.liveHermes.pid)).Run() != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		fixture.t.Error("Hermes fixture process survived cleanup")
	})
}

func (fixture *dashboardFixture) setupLiveHermes() {
	t := fixture.t
	executable := os.Getenv("A_GENT_HERMES_EXECUTABLE")
	if executable == "" {
		var err error
		executable, err = exec.LookPath("hermes")
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", fixture.root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(fixture.root, "cache"))
	config := filepath.Join(fixture.root, "hermes-config")
	t.Setenv("HERMES_HOME", config)
	t.Setenv("OPENAI_API_KEY", "local-fixture")
	t.Setenv("TERMINAL_CWD", fixture.root)
	t.Setenv("HERMES_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	fixture.liveHermes = &liveHermesFixture{release: make(chan struct{}, 1)}
	subagents := os.Getenv("A_GENT_TEST_HERMES_SUBAGENTS") == "1"
	var sequence atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"id": "fixture-model"}}})
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		index := sequence.Add(1)
		message := map[string]any{"role": "assistant", "content": fmt.Sprintf("hermes-live-completed-%d", index-1)}
		finish := "stop"
		if index == 1 {
			finish = "tool_calls"
			message["content"] = nil
			message["tool_calls"] = []any{map[string]any{"id": "fixture-edit", "type": "function",
				"function": map[string]string{"name": "patch", "arguments": `{"path":"fixture.txt","old_string":"old value","new_string":"verified Hermes change"}`}}}
		}
		if subagents && index == 1 {
			message["tool_calls"] = []any{map[string]any{
				"id": "fixture-delegate", "type": "function",
				"function": map[string]string{
					"name":      "delegate_task",
					"arguments": `{"tasks":[{"goal":"Hermes child fixture","toolsets":["file"]}]}`,
				},
			}}
		}
		var request struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode fixture request: %v", err)
			return
		}
		childRequest := false
		if subagents {
			for _, entry := range request.Messages {
				if entry.Role == "user" && entry.Content == "Hermes child fixture" {
					childRequest = true
				}
			}
			if childRequest {
				message["content"] = "HERMES CHILD PUBLIC OUTPUT"
			}
		}
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			if index == 1 {
				message["tool_calls"].([]any)[0].(map[string]any)["index"] = 0
			}
			for i, delta := range []map[string]any{message, {}} {
				var reason any
				if i == 1 {
					reason = finish
				}
				chunk := map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1,
					"model": "fixture-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if i == 0 && index != 1 && (!subagents || childRequest) {
					w.(http.Flusher).Flush()
					select {
					case <-fixture.liveHermes.release:
					case <-r.Context().Done():
						return
					}
				}
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		} else {
			if subagents && childRequest {
				select {
				case <-fixture.liveHermes.release:
				case <-r.Context().Done():
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1,
				"model": "fixture-model", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
				"usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}})
		}
	}))
	t.Cleanup(func() { close(fixture.liveHermes.release); model.Close() })
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	configuration := fmt.Sprintf(`_config_version: 12
model:
  default: fixture-model
  provider: custom
  base_url: %s/v1
  api_key: local-fixture
  api_mode: chat_completions
  context_length: 131072
display:
  interface: cli
  streaming: false
  compact: true
telemetry:
  shared_metrics:
    enabled: false
    send: false
    offer_version: 2
auxiliary:
  title_generation:
    enabled: false
memory:
  memory_enabled: false
  user_profile_enabled: false
agent:
  max_turns: 5
  reasoning_effort: none
terminal:
  backend: local
`, model.URL)
	if err := os.WriteFile(filepath.Join(config, "config.yaml"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.root, "fixture.txt"), []byte("old value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := hermes.ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	// Use an ordinary interactive shell launch: the normal plugin directory is
	// the only mechanism that loads the observer. No launcher-only status flags.
	fixture.tmux("-f", "/dev/null", "new-session", "-d", "-s", "hermes", "-x", "120", "-y", "32",
		"-c", fixture.root, "exec "+shellQuote(executable)+" chat --cli --ignore-rules --toolsets file,delegation")
	pid, err := strconv.Atoi(strings.TrimSpace(fixture.tmux("display-message", "-p", "-t", "hermes:0", "#{pane_pid}")))
	if err != nil {
		t.Fatal(err)
	}
	fixture.liveHermes.pid = pid
	t.Cleanup(func() {
		fixture.stopLiveHermes()
		_, _ = fixture.tmuxOutput("kill-server")
	})
	bin := filepath.Join(fixture.root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	// Discovery still asks the OS about the real PID each time. Limit it to this
	// isolated CLI so the test can never select the user's live Hermes sessions.
	script := fmt.Sprintf("#!/bin/sh\nexec /bin/ps -p %d -o pid=,lstart=,command=\n", pid)
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("A_GENT_TEST_HERMES", "1")
	fixture.tmux("set-environment", "-g", "A_GENT_TEST_HERMES", "1")
	fixture.tmux("set-environment", "-g", "PATH", os.Getenv("PATH"))
	adapter := hermes.NewAdapter()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		sessions, err := adapter.Sessions(t.Context())
		if err == nil && len(sessions) == 1 && sessions[0].TranscriptID != "" &&
			strings.Contains(fixture.tmux("capture-pane", "-p", "-t", "hermes:0"), "❯ ") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Hermes failed to load observer:\n%s", fixture.tmux("capture-pane", "-p", "-t", "hermes:0"))
}

func TestIntegrationPlainHermesUpdatesDashboard(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_HERMES_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_HERMES_TESTS=1 to test real Hermes")
	}
	fixture := newDashboardFixtureWithProvider(t, "hermes-live")
	fixture.key("g")
	fixture.waitText("│  Hermes ")
	fixture.waitText("Status     ● Idle")
	for index := 1; index <= 3; index++ {
		fixture.sendHermesInput("Edit fixture.txt and report completion.")
		fixture.waitFor("Hermes running", 15*time.Second, func(screen string) bool {
			return strings.Contains(screen, "Status     ● Running")
		})
		fixture.waitText(fmt.Sprintf("hermes-live-completed-%d", index))
		fixture.capture(fmt.Sprintf("hermes-live-%d", index))
		fixture.liveHermes.release <- struct{}{}
		fixture.waitText("Status     ● Idle")
		fixture.key("v")
		fixture.waitText("verified Hermes change")
		fixture.key("q")
		fixture.waitText("SELECTED SESSION")
	}
	fixture.capture("hermes-idle")
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.width, fixture.height = 80, 24
	fixture.waitText("v: expand")
	fixture.capture("hermes-narrow")
	fixture.key("v")
	fixture.waitText("EDIT PREVIEW · Esc/q")
	fixture.key("q")
	fixture.waitText("SELECTED SESSION")
	fixture.sendHermesInput("/new")
	fixture.waitFor("new Hermes session clears old preview", 4*time.Second, func(screen string) bool {
		return !strings.Contains(screen, "verified Hermes change") && !strings.Contains(screen, "hermes-live-completed-")
	})
}

func (fixture *dashboardFixture) sendHermesInput(text string) {
	fixture.t.Helper()
	fixture.tmux("send-keys", "-t", "hermes:0", "-l", text)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		screen := fixture.tmux("capture-pane", "-p", "-t", "hermes:0")
		if strings.Contains(screen, "❯ "+text) {
			// Hermes treats Enter within 50 ms of the last input event as a pasted newline.
			time.Sleep(100 * time.Millisecond)
			fixture.tmux("send-keys", "-t", "hermes:0", "Enter")
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	fixture.t.Fatal("Hermes did not render the fixture input")
}

func TestIntegrationPlainHermesSubagents(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_HERMES_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_HERMES_TESTS=1 to test real Hermes")
	}
	t.Setenv("A_GENT_TEST_HERMES_SUBAGENTS", "1")
	fixture := newDashboardFixtureWithProvider(t, "hermes-live")
	fixture.key("g")
	fixture.waitText("│  Hermes ")
	fixture.sendHermesInput("Delegate the child fixture task.")
	fixture.waitText("[subagent] Hermes child")
	fixture.key("j")
	fixture.waitText("Status     ● Running")
	fixture.capture("hermes-child-running")
	fixture.key("k")
	fixture.waitFor("parent preview excludes child output", 4*time.Second, func(screen string) bool {
		return !strings.Contains(screen, "HERMES CHILD PUBLIC OUTPUT")
	})
	fixture.key("j")
	fixture.liveHermes.release <- struct{}{}
	fixture.waitText("Status     ● Idle")
	fixture.waitText("HERMES CHILD PUBLIC OUTPUT")
	fixture.capture("hermes-child-idle")
	fixture.key("k")
	fixture.waitText("hermes-live-completed-3")
	fixture.sendHermesInput("/new")
	fixture.waitFor("Hermes confirms reset", 4*time.Second, func(screen string) bool {
		return strings.Contains(fixture.tmux("capture-pane", "-p", "-t", "hermes:0"), "Approve Once")
	})
	fixture.tmux("send-keys", "-t", "hermes:0", "1", "Enter")
	fixture.waitFor("new parent clears old child", 4*time.Second, func(screen string) bool {
		return !strings.Contains(screen, "[subagent]")
	})
}
