package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/claude"
	"a-gent/internal/codex"
	"a-gent/internal/polling"
	"a-gent/internal/ui"
)

func TestLiveProvidersReachDashboard(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_TESTS=1 to check installed providers")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updates := make(chan polling.Update)
	done := make(chan struct{})
	go func() {
		defer close(done)
		polling.Run(ctx, []agent.Adapter{codex.NewAdapter(), claude.NewAdapter()}, updates)
	}()
	defer func() { cancel(); <-done }()
	model := ui.NewModel(nil)
	seen := make(map[string]bool)
	for len(seen) < 2 {
		select {
		case update, ok := <-updates:
			if !ok {
				t.Fatal("polling stopped before both providers responded")
			}
			if update.Err != nil {
				t.Fatalf("%s: %v", update.Provider, update.Err)
			}
			updated, _ := model.Update(update)
			model = updated.(ui.Model)
			seen[update.Provider] = true
			t.Logf("%s: %d live sessions", update.Provider, len(update.Sessions))
		case <-ctx.Done():
			t.Fatal("providers did not respond")
		}
	}
	view := model.View()
	if !strings.Contains(view, "codex") || !strings.Contains(view, "claude") {
		t.Fatal("dashboard did not render both providers")
	}
}
