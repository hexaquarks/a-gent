//go:build integration

package ui_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type piFixtureState struct {
	sync.Mutex
	started time.Time
	record  map[string]any
}

func (fixture *dashboardFixture) piDirectory() string {
	cache := filepath.Join(fixture.root, "cache")
	if runtime.GOOS == "darwin" {
		cache = filepath.Join(fixture.root, "Library", "Caches")
	}
	return filepath.Join(cache, "a-gent", "pi", "instances")
}

func (fixture *dashboardFixture) setupPi() {
	t := fixture.t
	fixture.pi = &piFixtureState{started: time.Now()}
	bin := filepath.Join(fixture.root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	// Only process discovery is substituted. Status and selected-session previews
	// use the production adapter and native-shaped observer/transcript records.
	process := fmt.Sprintf("%d %s node /fixture/@earendil-works/pi-coding-agent/dist/bundle/cli.js\n",
		os.Getpid(), time.Now().Format("Mon Jan _2 15:04:05 2006"))
	path := filepath.Join(fixture.root, "pi-process")
	if err := os.WriteFile(path, []byte(process), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\ncat "+shellQuote(path)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", fixture.root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(fixture.root, "cache"))
	t.Setenv("A_GENT_TEST_PI", "1")
	fixture.transcripts["pi"] = filepath.Join(fixture.root, "pi-session.jsonl")
	fixture.writePiTranscript("pi-update-0")
	fixture.writePiState("running", "current")
	// The test controls status transitions; a heartbeat keeps unchanged metadata
	// fresh without inventing new activity or rewriting the transcript.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				fixture.writePiState("", "")
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
}

func (fixture *dashboardFixture) writePiState(state, leaf string) {
	fixture.t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		fixture.t.Fatal(err)
	}
	fixture.pi.Lock()
	defer fixture.pi.Unlock()
	if state != "" {
		fixture.pi.record = map[string]any{"version": 1, "pid": os.Getpid(),
			"startedAt": fixture.pi.started.UnixMilli(), "lastActiveAt": time.Now().Add(time.Second).UnixMilli(),
			"sessionID": "pi-fixture", "sessionFile": fixture.transcripts["pi"], "leafID": leaf,
			"directory": directory, "name": "Pi fixture", "state": state}
	}
	fixture.pi.record["updatedAt"] = time.Now().UnixMilli()
	data, _ := json.Marshal(fixture.pi.record)
	if err := os.MkdirAll(fixture.piDirectory(), 0700); err != nil {
		fixture.t.Fatal(err)
	}
	path := filepath.Join(fixture.piDirectory(), fmt.Sprintf("%d.json", os.Getpid()))
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		fixture.t.Fatal(err)
	}
}

func (fixture *dashboardFixture) writePiTranscript(output string) {
	fixture.t.Helper()
	directory, _ := os.Getwd()
	file, err := os.Create(fixture.transcripts["pi"])
	if err != nil {
		fixture.t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.Encode(map[string]any{"type": "session", "version": 3, "id": "pi-fixture", "cwd": directory})
	entry := func(id, parent, role string, content []any, extra map[string]any) {
		message := map[string]any{"role": role, "content": content}
		for key, value := range extra {
			message[key] = value
		}
		encoder.Encode(map[string]any{"type": "message", "id": id, "parentId": parent,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "message": message})
	}
	entry("request", "", "assistant", []any{map[string]any{"type": "toolCall", "id": "edit-one", "name": "edit",
		"arguments": map[string]any{"path": "fixture.go"}}}, nil)
	entry("result", "request", "toolResult", []any{map[string]string{"type": "text", "text": "Edit applied"}},
		map[string]any{
			"toolCallId": "edit-one", "toolName": "edit", "isError": false,
			"details": map[string]string{"diff": "-1 old value\n+1 verified Pi change"},
		})
	entry("abandoned", "result", "assistant",
		[]any{map[string]string{"type": "text", "text": "ABANDONED_MUST_STAY_HIDDEN"}}, nil)
	entry("current", "result", "assistant", []any{map[string]string{"type": "text", "text": output},
		map[string]string{"type": "thinking", "thinking": "PRIVATE_MUST_STAY_HIDDEN"}}, nil)
}

func TestIntegrationPiStatusRealtimePreviewAndBranchIsolation(t *testing.T) {
	fixture := newDashboardFixtureWithProvider(t, "pi")
	fixture.key("g")
	fixture.waitText("pi-update-0")
	for sequence := 1; sequence <= 3; sequence++ {
		output := fmt.Sprintf("pi-update-%d", sequence)
		fixture.writePiTranscript(output)
		screen := fixture.waitText(output)
		if strings.Contains(screen, "PRIVATE_MUST_STAY_HIDDEN") || strings.Contains(screen, "ABANDONED_MUST_STAY_HIDDEN") {
			t.Fatalf("Pi leaked reasoning or abandoned branch:\n%s", screen)
		}
	}
	fixture.capture("pi-live")
	fixture.writePiState("idle", "current")
	fixture.waitProviderState("pi", "idle")
	fixture.waitText("verified Pi change")
	fixture.capture("pi-idle")
	fixture.writePiState("unknown", "current")
	fixture.waitText("unavailable")
	fixture.writePiState("idle", "current")
	fixture.waitProviderState("pi", "idle")
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.width, fixture.height = 80, 24
	fixture.waitFor("narrow Pi preview", 4*time.Second, func(screen string) bool {
		return strings.HasPrefix(screen, "╭") && strings.Contains(screen, "v expand") &&
			strings.Contains(screen, "fixture.go") &&
			!strings.Contains(screen, "│  LIVE ACTIVITY")
	})
	fixture.capture("pi-narrow")
	fixture.key("v")
	fixture.waitText("EDIT PREVIEW · Esc/q")
	fixture.key("q")
	fixture.waitText("ACTIVITY")
}
