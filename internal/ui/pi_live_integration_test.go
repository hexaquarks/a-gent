//go:build integration

package ui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"a-gent/internal/pi"
)

type livePiFixture struct {
	input   io.WriteCloser
	release chan struct{}
}

func (fixture *dashboardFixture) setupLivePi() {
	t := fixture.t
	executable := os.Getenv("A_GENT_PI_EXECUTABLE")
	if executable == "" {
		var err error
		executable, err = exec.LookPath("pi")
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", fixture.root)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(fixture.root, "cache"))
	config := filepath.Join(fixture.root, "pi-config")
	t.Setenv("PI_CODING_AGENT_DIR", config)
	if _, err := pi.ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	fixture.livePi = &livePiFixture{release: make(chan struct{}, 1)}
	var sequence atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		index := sequence.Add(1)
		select {
		case <-fixture.livePi.release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for i, delta := range []map[string]any{
			{"role": "assistant", "content": fmt.Sprintf("pi-live-completed-%d", index)}, {},
		} {
			var finish any
			if i == 1 {
				finish = "stop"
			}
			data, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1,
				"model": "deterministic", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		close(fixture.livePi.release)
		model.Close()
	})
	data, _ := json.Marshal(map[string]any{"providers": map[string]any{"fixture": map[string]any{
		"baseUrl": model.URL + "/v1", "api": "openai-completions", "apiKey": "local-fixture",
		"models": []any{map[string]any{"id": "deterministic", "name": "Fixture", "reasoning": false,
			"input": []string{"text"}, "contextWindow": 8192, "maxTokens": 1024,
			"cost": map[string]int{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0}}},
	}}})
	if err := os.WriteFile(filepath.Join(config, "models.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	// No --extension argument: this reproduces an ordinary shell launch.
	command := exec.CommandContext(ctx, executable, "--mode", "rpc", "--name", "Live Pi fixture",
		"--provider", "fixture", "--model", "deterministic", "--thinking", "off", "--no-tools")
	command.Dir = fixture.root
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	fixture.livePi.input = input
	logFile, err := os.Create(filepath.Join(fixture.root, "live-pi.log"))
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
	// Filter discovery to the real fixture process, so the dashboard cannot read
	// the user's sessions. Runtime metadata and transcripts are untouched.
	process, err := exec.Command("ps", "-p", fmt.Sprint(command.Process.Pid), "-o", "pid=,lstart=,command=").Output()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(fixture.root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.root, "pi-process")
	if err := os.WriteFile(path, process, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\ncat "+shellQuote(path)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("A_GENT_TEST_PI", "1")
	fixture.promptLivePi()
}

func (fixture *dashboardFixture) promptLivePi() {
	fixture.t.Helper()
	if _, err := fmt.Fprintln(fixture.livePi.input, `{"type":"prompt","message":"Reply with the fixture marker."}`); err != nil {
		fixture.t.Fatal(err)
	}
}

func TestIntegrationPlainPiUpdatesDashboard(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_PI_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_PI_TESTS=1 to test real Pi in the dashboard")
	}
	fixture := newDashboardFixtureWithProvider(t, "pi-live")
	fixture.waitText("Live Pi fixture")
	fixture.key("g")
	fixture.waitText("│  Live Pi fixture ")
	for index := 1; index <= 3; index++ {
		if index > 1 {
			fixture.promptLivePi()
		}
		fixture.waitProviderState("pi", "running")
		fixture.livePi.release <- struct{}{}
		screen := fixture.waitText(fmt.Sprintf("pi-live-completed-%d", index))
		if strings.Contains(screen, "unavailable") {
			t.Fatalf("plain Pi unavailable:\n%s", screen)
		}
		fixture.waitProviderState("pi", "idle")
	}
	fixture.capture("plain-pi-live-output")
}
