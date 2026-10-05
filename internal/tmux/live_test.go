package tmux

import (
	"context"
	"os"
	"testing"
	"time"

	"a-gent/internal/claude"
)

// Run with Claude Code open inside tmux. Checks which pane would open without
// switching windows or sending input to Claude.
func TestLiveClaudePaneResolution(t *testing.T) {
	if os.Getenv("A_GENT_LIVE_TESTS") != "1" {
		t.Skip("set A_GENT_LIVE_TESTS=1 to check local Claude/tmux integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sessions, err := claude.NewAdapter().Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	navigator := Navigator{clientName: "test", processParents: readProcessParents}
	var target string
	navigator.runCommand = func(ctx context.Context, command string, arguments ...string) ([]byte, error) {
		if command == "switch-client" {
			target = arguments[len(arguments)-1]
			return nil, nil
		}
		return runTmuxCommand(ctx, command, arguments...)
	}
	for _, session := range sessions {
		if session.ProcessID == nil || *session.ProcessID <= 0 {
			continue
		}
		if err := navigator.Navigate(ctx, session); err == nil {
			t.Logf("Resolved live Claude session (%s) to pane %s without switching", session.State, target)
			return
		}
	}
	t.Fatal("no live Claude session could be resolved to a tmux pane")
}
