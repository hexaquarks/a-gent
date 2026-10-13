//go:build integration

package ui_test

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type liveCodexFixture struct {
	connection net.Conn
	reader     *bufio.Reader
	release    chan struct{}
	nextID     int
	stop       func()
}

func (fixture *dashboardFixture) setupLiveCodex() {
	t := fixture.t
	executable, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}

	// A short, isolated home also keeps the daemon's Unix socket within macOS limits.
	home, err := os.MkdirTemp("/tmp", "ag-codex-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("CODEX_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("A_GENT_TEST_CODEX_LIVE", "1")
	fixture.liveCodex = &liveCodexFixture{release: make(chan struct{}), nextID: 1}

	var spawned bool
	var mutex sync.Mutex
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var request struct {
			Tools []struct {
				Name  string `json:"name"`
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		spawnNamespace := ""
		for _, tool := range request.Tools {
			for _, nested := range tool.Tools {
				if nested.Name == "spawn_agent" {
					spawnNamespace = tool.Name
				}
			}
		}

		mutex.Lock()
		spawn := !spawned
		spawned = true
		mutex.Unlock()

		child := strings.Contains(string(body), "CHILD_TASK_MARKER") &&
			!strings.Contains(string(body), "function_call_output")
		if child && !spawn {
			select {
			case <-fixture.liveCodex.release:
			case <-r.Context().Done():
				return
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		events := []map[string]any{{"type": "response.created", "response": map[string]any{"id": "fixture-response"}}}
		item := map[string]any{
			"type": "message", "role": "assistant", "id": "fixture-message",
			"content": []any{map[string]any{"type": "output_text", "text": "PARENT_PUBLIC_OUTPUT"}},
		}
		if spawn {
			item = map[string]any{
				"type": "function_call", "call_id": "spawn-fixture", "name": "spawn_agent",
				"arguments": `{"task_name":"child","message":"CHILD_TASK_MARKER","fork_turns":"none"}`,
			}
			if spawnNamespace != "" {
				item["namespace"] = spawnNamespace
			}
		} else if child {
			item["content"] = []any{map[string]any{"type": "output_text", "text": "CHILD_PUBLIC_OUTPUT"}}
		}
		events = append(events, map[string]any{"type": "response.output_item.done", "item": item}, map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    "fixture-response",
				"usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
			},
		})
		for _, event := range events {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	t.Cleanup(model.Close)
	config := fmt.Sprintf(`model = "gpt-5.4"
model_provider = "fixture"
approval_policy = "never"
sandbox_mode = "read-only"
[features]
multi_agent = true
[model_providers.fixture]
name = "Local fixture"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
`, model.URL+"/v1")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(home, "server.sock")
	// Launch a real, owned app-server rather than starting or altering the user's daemon.
	server := exec.Command(executable, "app-server", "--listen", "unix://"+socket)
	logFile, err := os.Create(filepath.Join(home, "codex.log"))
	if err != nil {
		t.Fatal(err)
	}
	server.Stdout, server.Stderr = logFile, logFile
	if err := server.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	var stopOnce sync.Once
	fixture.liveCodex.stop = func() {
		stopOnce.Do(func() {
			server.Process.Kill()
			server.Wait()
			logFile.Close()
			if t.Failed() {
				data, _ := os.ReadFile(logFile.Name())
				t.Logf("Codex log:\n%s", data)
			}
		})
	}
	t.Cleanup(fixture.liveCodex.stop)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Codex socket did not appear")
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Only the daemon location command is redirected; thread discovery and
	// previews use the real server's production protocol and native records.
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	version, _ := json.Marshal(map[string]string{"socketPath": socket, "status": "running"})
	wrapper := "#!/bin/sh\nif [ \"$*\" = \"app-server daemon version\" ]; then\n" +
		" printf '%s\\n' " + shellQuote(string(version)) + "\nelse\n" +
		" exec " + shellQuote(executable) + " \"$@\"\nfi\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	connection, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fixture.liveCodex.connection = connection
	fixture.liveCodex.reader = bufio.NewReader(connection)
	t.Cleanup(func() { connection.Close() })
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(connection, "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\n"+
		"Connection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n")
	response, err := http.ReadResponse(fixture.liveCodex.reader, &http.Request{Method: "GET"})
	if err != nil || response.StatusCode != 101 {
		t.Fatalf("Codex handshake: %v, %v", response, err)
	}
	fixture.codexRequest("initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "a-gent-test", "version": "1"},
		"capabilities": map[string]bool{"experimentalApi": true},
	})
	fixture.writeCodexJSON([]byte(`{"method":"initialized","params":{}}`))
	result := fixture.codexRequest("thread/start", map[string]any{"cwd": fixture.root})
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &started); err != nil {
		t.Fatal(err)
	}
	fixture.codexRequest("thread/name/set", map[string]any{"threadId": started.Thread.ID, "name": "Codex parent fixture"})
	fixture.codexRequest("turn/start", map[string]any{
		"threadId": started.Thread.ID,
		"input": []any{map[string]any{
			"type": "text", "text": "Spawn the fixture child.", "text_elements": []any{},
		}},
	})
}

func (fixture *dashboardFixture) codexRequest(method string, params any) json.RawMessage {
	fixture.t.Helper()
	live := fixture.liveCodex
	id := live.nextID
	live.nextID++
	data, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	fixture.writeCodexJSON(data)
	if err := live.connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		fixture.t.Fatal(err)
	}
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(live.reader, header); err != nil {
			fixture.t.Fatal(err)
		}
		length := uint64(header[1] & 127)
		if length == 126 {
			var value uint16
			if err := binary.Read(live.reader, binary.BigEndian, &value); err != nil {
				fixture.t.Fatal(err)
			}
			length = uint64(value)
		} else if length == 127 {
			if err := binary.Read(live.reader, binary.BigEndian, &length); err != nil {
				fixture.t.Fatal(err)
			}
		}
		if length > 4*1024*1024 {
			fixture.t.Fatal("Codex response too large")
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(live.reader, payload); err != nil {
			fixture.t.Fatal(err)
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(payload, &response); err != nil {
			fixture.t.Fatal(err)
		}
		if response.ID != id {
			continue
		}
		if len(response.Error) > 0 {
			fixture.t.Fatalf("Codex %s: %s", method, response.Error)
		}
		return response.Result
	}
}

// Small test-only WebSocket writer; the production adapter owns its connection.
func (fixture *dashboardFixture) writeCodexJSON(data []byte) {
	fixture.t.Helper()
	header := []byte{0x81}
	if len(data) < 126 {
		header = append(header, 0x80|byte(len(data)))
	} else {
		header = append(header, 0x80|126)
		header = binary.BigEndian.AppendUint16(header, uint16(len(data)))
	}
	mask := []byte{1, 2, 3, 4}
	header = append(header, mask...)
	for index := range data {
		data[index] ^= mask[index%4]
	}
	if _, err := fixture.liveCodex.connection.Write(append(header, data...)); err != nil {
		fixture.t.Fatal(err)
	}
}

func TestIntegrationCodexSubagents(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_CODEX_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_CODEX_TESTS=1 to test installed Codex subagents")
	}
	fixture := newDashboardFixtureWithProvider(t, "codex-live")
	fixture.waitText("↳")
	fixture.capture("child-running")
	fixture.key("g")
	fixture.key("j")
	fixture.waitText("Status     ● Running")
	close(fixture.liveCodex.release)
	fixture.waitText("CHILD_PUBLIC_OUTPUT")
	fixture.waitText("Status     ● Idle")
	fixture.capture("child-idle")
	fixture.key("k")
	screen := fixture.waitText("PARENT_PUBLIC_OUTPUT")
	if strings.Contains(screen, "CHILD_PUBLIC_OUTPUT") {
		t.Fatalf("parent shows child preview:\n%s", screen)
	}
	fixture.capture("parent-preview")
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "70", "-y", "25")
	fixture.key("j")
	fixture.waitText("CHILD_PUBLIC_OUTPUT")
	fixture.waitText("↳codex")
	fixture.capture("child-narrow")

	loaded := fixture.codexRequest("thread/loaded/list", map[string]any{})
	var ids struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(loaded, &ids); err != nil {
		t.Fatal(err)
	}
	childID := ""
	for _, id := range ids.Data {
		result := fixture.codexRequest("thread/read", map[string]any{"threadId": id, "includeTurns": false})
		var response struct {
			Thread struct {
				ParentID string `json:"parentThreadId"`
			} `json:"thread"`
		}
		if err := json.Unmarshal(result, &response); err != nil {
			t.Fatal(err)
		}
		if response.Thread.ParentID != "" {
			childID = id
			break
		}
	}
	if childID == "" {
		t.Fatal("real Codex returned no child relationship")
	}
	fixture.codexRequest("thread/archive", map[string]any{"threadId": childID})
	fixture.waitFor("closed child disappears", 5*time.Second, func(screen string) bool {
		return strings.Contains(screen, "2 sessions") && !strings.Contains(screen, "↳")
	})
	fixture.capture("child-closed")
}
