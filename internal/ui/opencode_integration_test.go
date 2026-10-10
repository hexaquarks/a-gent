//go:build integration

package ui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Only process metadata is replaced. The production adapter discovers the real
// fixture listener with lsof and reads native HTTP records through the UI loop.
// This prevents tests from querying any user's real OpenCode conversations.
func (fixture *dashboardFixture) setupOpenCode() {
	t := fixture.t
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	state := openCodeFixtureState(directory, "busy", false)
	fixture.writeOpenCodeState(state)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("directory") != directory {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		data, err := os.ReadFile(filepath.Join(fixture.root, "opencode-api.json"))
		if err != nil {
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
			return
		}
		var responses map[string]json.RawMessage
		if err := json.Unmarshal(data, &responses); err != nil {
			http.Error(w, "invalid fixture", http.StatusInternalServerError)
			return
		}
		response, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(response)
	}))
	t.Cleanup(server.Close)
	bin := filepath.Join(fixture.root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	process := fmt.Sprintf("%d %s opencode --port 0\n", os.Getpid(), time.Now().Format("Mon Jan _2 15:04:05 2006"))
	// lsof uses this actual PID, directory, and ephemeral loopback listener.
	record := filepath.Join(fixture.root, "process-record")
	if err := os.WriteFile(record, []byte(process), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\ncat "+shellQuote(record)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("A_GENT_TEST_OPENCODE", "1")
	t.Setenv("OPENCODE_SERVER_PASSWORD", "")
}

func openCodeFixtureState(directory, status string, waiting bool) map[string]any {
	timestamp := time.Now().Add(time.Second).UnixMilli()
	statuses := map[string]any{}
	if status != "idle" {
		statuses["ses_fixture"] = map[string]string{"type": status}
	}
	permission := []any{}
	if waiting {
		permission = append(permission, map[string]string{"sessionID": "ses_fixture"})
	}
	return map[string]any{
		"/session": []any{
			map[string]any{"id": "ses_fixture", "title": "OpenCode fixture", "directory": directory,
				"time": map[string]int64{"created": timestamp, "updated": timestamp}},
			map[string]any{"id": "ses_history", "title": "HISTORY_MUST_STAY_HIDDEN", "directory": directory,
				"time": map[string]int64{"created": timestamp, "updated": timestamp}},
		},
		"/session/status": statuses,
		"/permission":     permission,
		"/question":       []any{},
	}
}

func (fixture *dashboardFixture) writeOpenCodeState(state map[string]any) {
	fixture.t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		fixture.t.Fatal(err)
	}
	path := filepath.Join(fixture.root, "opencode-api.json")
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		fixture.t.Fatal(err)
	}
}

func TestIntegrationOpenCodeStatus(t *testing.T) {
	fixture := newDashboardFixtureWithOpenCode(t, true)
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fixture.key("g")
	screen := fixture.waitText("OpenCode fixture")
	if strings.Contains(screen, "HISTORY_MUST_STAY_HIDDEN") {
		t.Fatalf("OpenCode leaked session history:\n%s", screen)
	}
	fixture.waitText("3 running")
	fixture.capture("opencode-running")
	fixture.writeOpenCodeState(openCodeFixtureState(directory, "busy", true))
	fixture.waitText("Needs input")
	fixture.capture("opencode-waiting")
	fixture.writeOpenCodeState(openCodeFixtureState(directory, "idle", false))
	fixture.waitText("Status     ● Idle")
	fixture.capture("opencode-idle")
	state := openCodeFixtureState(directory, "idle", false)
	state["/session/status"] = nil
	fixture.writeOpenCodeState(state)
	fixture.waitText("Unavailable")
	fixture.capture("opencode-unavailable")
	fixture.writeOpenCodeState(openCodeFixtureState(directory, "idle", false))
	fixture.waitText("Status     ● Idle")
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.width, fixture.height = 80, 24
	fixture.waitText("OpenCode fixture")
	fixture.capture("opencode-narrow")
}
