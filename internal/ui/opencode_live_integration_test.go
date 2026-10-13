//go:build integration

package ui_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"a-gent/internal/opencode"
)

type liveOpenCodeFixture struct {
	release chan struct{}
	pid     int
	stop    sync.Once
}

func (fixture *dashboardFixture) stopLiveOpenCode() {
	if fixture.liveOpenCode == nil {
		return
	}
	fixture.liveOpenCode.stop.Do(func() {
		pid := fixture.liveOpenCode.pid
		if fixture.t.Failed() {
			screen, _ := fixture.tmuxOutput("capture-pane", "-p", "-t", "opencode:0")
			fixture.t.Logf("OpenCode terminal:\n%s", screen)
		}
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if exec.Command("/bin/ps", "-p", strconv.Itoa(pid)).Run() != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		fixture.t.Error("OpenCode fixture process survived cleanup")
	})
}

func (fixture *dashboardFixture) setupLiveOpenCode(launcherFlags bool) {
	t := fixture.t
	project, err := filepath.EvalSymlinks(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	fixture.root = project
	executable, err := exec.LookPath("opencode")
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"HOME":                            fixture.root,
		"XDG_CONFIG_HOME":                 filepath.Join(fixture.root, "config"),
		"XDG_DATA_HOME":                   filepath.Join(fixture.root, "data"),
		"XDG_CACHE_HOME":                  filepath.Join(fixture.root, "cache"),
		"OPENCODE_CONFIG_DIR":             filepath.Join(fixture.root, "config", "opencode"),
		"OPENCODE_SERVER_PASSWORD":        "",
		"OPENCODE_DISABLE_AUTOUPDATE":     "1",
		"OPENCODE_DISABLE_PROJECT_CONFIG": "1",
		"OPENCODE_DISABLE_MODELS_FETCH":   "1",
	} {
		t.Setenv(key, value)
	}
	if err := opencode.ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	fixture.liveOpenCode = &liveOpenCodeFixture{release: make(chan struct{})}
	var spawned bool
	var mutex sync.Mutex
	childStarted := make(chan struct{})
	var startOnce sync.Once
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, "invalid fixture model request", http.StatusBadRequest)
			return
		}
		child := false
		for _, message := range request.Messages {
			child = child || (message.Role == "user" && strings.Contains(string(message.Content), "CHILD_TASK_MARKER"))
		}
		taskAvailable := false
		for _, tool := range request.Tools {
			taskAvailable = taskAvailable || tool.Function.Name == "task"
		}
		mutex.Lock()
		spawn := !spawned && taskAvailable && !child
		spawned = spawned || spawn
		mutex.Unlock()
		if child {
			startOnce.Do(func() { close(childStarted) })
			select {
			case <-fixture.liveOpenCode.release:
			case <-r.Context().Done():
				return
			}
		}
		delta := map[string]any{"role": "assistant", "content": "PARENT_PUBLIC_OUTPUT"}
		finish := "stop"
		if spawn {
			finish = "tool_calls"
			delta = map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"index": 0, "id": "fixture-task", "type": "function",
					"function": map[string]string{
						"name":      "task",
						"arguments": `{"description":"OpenCode child fixture","prompt":"CHILD_TASK_MARKER","subagent_type":"general"}`,
					},
				}},
			}
		} else if child {
			delta["content"] = "CHILD_PUBLIC_OUTPUT"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []map[string]any{
			{"index": 0, "delta": delta, "finish_reason": nil},
			{"index": 0, "delta": map[string]any{}, "finish_reason": finish},
		} {
			data, _ := json.Marshal(map[string]any{
				"id": "fixture", "object": "chat.completion.chunk", "created": 1,
				"model": "deterministic", "choices": []any{chunk},
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(model.Close)
	config, err := json.Marshal(map[string]any{
		"model": "fixture/deterministic", "small_model": "fixture/deterministic",
		"permission": map[string]string{"task": "allow"},
		"agent":      map[string]any{"title": map[string]bool{"disable": true}},
		"provider": map[string]any{"fixture": map[string]any{
			"npm": "@ai-sdk/openai-compatible", "name": "Fixture",
			"options": map[string]string{"baseURL": model.URL + "/v1", "apiKey": "local-fixture"},
			"models": map[string]any{"deterministic": map[string]any{
				"name": "Deterministic", "limit": map[string]int{"context": 8192, "output": 1024},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCODE_CONFIG_CONTENT", string(config))
	command := "exec " + shellQuote(executable)
	if launcherFlags {
		command += " --port 0 --hostname 127.0.0.1"
	}
	command += " --prompt " + shellQuote("OpenCode parent fixture: delegate the fixture task.")
	fixture.tmux("-f", "/dev/null", "new-session", "-d", "-s", "opencode", "-x", "120", "-y", "32",
		"-c", fixture.root, command)
	pid, err := strconv.Atoi(strings.TrimSpace(fixture.tmux("display-message", "-p", "-t", "opencode:0", "#{pane_pid}")))
	if err != nil {
		t.Fatal(err)
	}
	fixture.liveOpenCode.pid = pid
	t.Cleanup(func() {
		fixture.stopLiveOpenCode()
		_, _ = fixture.tmuxOutput("kill-server")
	})
	bin := filepath.Join(fixture.root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	// Only OS process discovery is restricted. The installed CLI executes Task,
	// creates native child sessions, and serves its own status and messages.
	// Unlike ps -A, ps -p exits 1 when the process is gone. Treat that as an
	// empty successful discovery result, matching the production command.
	script := fmt.Sprintf("#!/bin/sh\n/bin/ps -p %d -o pid=,lstart=,command= || test \"$?\" = 1\n", pid)
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("A_GENT_TEST_OPENCODE", "1")
	fixture.tmux("set-environment", "-g", "A_GENT_TEST_OPENCODE", "1")
	fixture.tmux("set-environment", "-g", "PATH", os.Getenv("PATH"))
	select {
	case <-childStarted:
	case <-time.After(25 * time.Second):
		t.Fatal("installed OpenCode did not start the native child task")
	}
}

func TestIntegrationOpenCodeSubagents(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_OPENCODE_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_OPENCODE_TESTS=1 to test installed OpenCode subagents")
	}
	for _, mode := range []string{"plain", "launcher"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newDashboardFixtureWithProvider(t, "opencode-live-"+mode)
			fixture.waitText("[subagent]")
			selectedTitle := func(screen string) string {
				details := strings.SplitN(screen, "SELECTED SESSION", 2)
				if len(details) != 2 {
					return ""
				}
				rows := strings.SplitN(details[1], "\n", 3)
				if len(rows) < 2 {
					return ""
				}
				return rows[1]
			}
			fixture.key("g")
			screen := fixture.waitFor("first selected row", 4*time.Second, func(screen string) bool {
				rows := strings.Split(screen, "\n")
				for index, row := range rows {
					if strings.Contains(row, "Agent") && strings.Contains(row, "Session") && index+1 < len(rows) {
						return strings.Contains(rows[index+1], "›")
					}
				}
				return false
			})
			// Walk to the child using visible selection changes, so a slow terminal
			// cannot accidentally queue extra navigation keys past its row.
			for range 4 {
				previousTitle := selectedTitle(screen)
				if strings.Contains(previousTitle, "[subagent]") {
					break
				}
				fixture.key("j")
				screen = fixture.waitFor("next selected session", 4*time.Second, func(screen string) bool {
					return selectedTitle(screen) != "" && selectedTitle(screen) != previousTitle
				})
			}
			if !strings.Contains(selectedTitle(screen), "[subagent]") {
				t.Fatalf("native child was not selectable:\n%s", screen)
			}
			fixture.waitText("Status     ● running")
			fixture.capture("child-running")
			close(fixture.liveOpenCode.release)
			fixture.waitText("CHILD_PUBLIC_OUTPUT")
			fixture.waitText("Status     ● idle")
			fixture.capture("child-idle")
			fixture.key("k")
			screen = fixture.waitText("PARENT_PUBLIC_OUTPUT")
			if strings.Contains(screen, "CHILD_PUBLIC_OUTPUT") {
				t.Fatalf("parent preview contains child output:\n%s", screen)
			}
			fixture.capture("parent-preview")
			fixture.key("m")
			screen = fixture.waitFor("Board columns", 4*time.Second, func(screen string) bool {
				return strings.Contains(screen, "running") && strings.Contains(screen, "needs you")
			})
			if strings.Contains(screen, "[subagent]") {
				t.Fatalf("Board includes a subagent:\n%s", screen)
			}
			fixture.key("m")
			fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "70", "-y", "25")
			fixture.width, fixture.height = 70, 25
			fixture.key("j")
			fixture.waitText("CHILD_PUBLIC_OUTPUT")
			fixture.capture("child-narrow")
			fixture.stopLiveOpenCode()
			fixture.waitFor("exited process and children disappear", 5*time.Second, func(screen string) bool {
				return strings.Contains(screen, "2 sessions") && !strings.Contains(screen, "[subagent]")
			})
			fixture.capture("process-exited")
		})
	}
}
