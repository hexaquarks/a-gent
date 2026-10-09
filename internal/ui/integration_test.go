//go:build integration

package ui_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestIntegrationFeedUpdatesInRealtime(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			fixture := newDashboardFixture(t)
			fixture.selectProvider(provider)
			previous := ""
			// Only append transcript records: no restart, selection changes, metadata
			// changes, or explicit UI refresh. This exercises the production refresh loop.
			for sequence := 1; sequence <= 3; sequence++ {
				marker := fmt.Sprintf("%s-update-%d", provider, sequence)
				started := time.Now()
				fixture.appendOutput(provider, marker)
				fixture.waitFor("live output "+marker, 4*time.Second, func(screen string) bool {
					return strings.Contains(screen, marker) && (previous == "" || !strings.Contains(screen, previous))
				})
				t.Logf("%s appeared in %s", marker, time.Since(started).Round(time.Millisecond))
				fixture.capture(fmt.Sprintf("update-%d", sequence))
				previous = marker
			}
		})
	}
}

func TestIntegrationCompletedDiffSurvivesIdleAndReadFailure(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.appendEdit("completed", "verified.go")
	fixture.appendEdit("failed", "failed.go")
	fixture.setState("codex", agent.StateIdle)
	screen := fixture.waitText("verified.go")
	if strings.Contains(screen, "failed.go") || !strings.Contains(screen, "+verified change") {
		t.Fatalf("wrong completed edit:\n%s", screen)
	}
	assertPreviewHeading(t, screen, "LAST EDIT")
	fixture.capture("idle-diff")
	fixture.key("v")
	fixture.waitText("EDIT PREVIEW · Esc/q")
	fixture.key("q")
	fixture.waitText("SELECTED SESSION")
	path := fixture.transcripts["codex"]
	if err := os.Rename(path, path+".hidden"); err != nil {
		t.Fatal(err)
	}
	screen = fixture.waitText("stale")
	if !strings.Contains(screen, "verified.go") {
		t.Fatalf("read failure discarded cached edit:\n%s", screen)
	}
	fixture.capture("stale-diff")
}

func TestIntegrationSessionSwitchExpansionAndLayout(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.appendOutput("codex", "CODEX_ONLY")
	fixture.appendOutput("claude", "CLAUDE_ONLY")
	fixture.waitText("CODEX_ONLY")
	fixture.selectProvider("claude")
	screen := fixture.waitText("CLAUDE_ONLY")
	if strings.Contains(screen, "CODEX_ONLY") {
		t.Fatal("session switch retained another provider's output")
	}
	assertPreviewHeading(t, screen, "LIVE ACTIVITY")
	assertProjectColumn(t, screen, true)
	fixture.capture("wide-activity")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	screen = fixture.waitText("v · Assistant: CLAUDE_ONLY")
	assertProjectColumn(t, screen, false)
	fixture.capture("narrow-activity")
	fixture.key("v")
	screen = fixture.waitText("LIVE ACTIVITY · Esc/q")
	if !strings.Contains(screen, "CLAUDE_ONLY") {
		t.Fatal("expanded feed lost the selected output")
	}
	fixture.capture("expanded-activity")
	fixture.key("q")
	fixture.waitText("SELECTED SESSION")
	fixture.selectProvider("codex")
	screen = fixture.waitText("CODEX_ONLY")
	if strings.Contains(screen, "CLAUDE_ONLY") {
		t.Fatal("returning to a cached session mixed providers")
	}
}

func assertProjectColumn(t *testing.T, screen string, visible bool) {
	t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "Agent") && strings.Contains(line, "Last active") {
			if strings.Contains(line, "Project") != visible {
				t.Fatalf("project column visibility should be %t:\n%s", visible, screen)
			}
			return
		}
	}
	t.Fatalf("session table header is missing:\n%s", screen)
}

func TestIntegrationEmptyPreviewStates(t *testing.T) {
	fixture := newDashboardFixture(t)
	screen := fixture.waitText("Waiting for output")
	assertPreviewHeading(t, screen, "PREVIEW")
	fixture.capture("waiting-for-output")
	path := fixture.transcripts["codex"]
	if err := os.Rename(path, path+".hidden"); err != nil {
		t.Fatal(err)
	}
	screen = fixture.waitText("Preview unavailable")
	assertPreviewHeading(t, screen, "PREVIEW")
	for _, redundant := range []string{"Checked ", "EDIT PREVIEW", "LIVE ACTIVITY", "v: expand"} {
		if strings.Contains(screen, redundant) {
			t.Fatalf("unavailable preview contains redundant content %q", redundant)
		}
	}
	fixture.capture("unavailable")
	if err := os.Rename(path+".hidden", path); err != nil {
		t.Fatal(err)
	}
	fixture.setState("codex", agent.StateIdle)
	screen = fixture.waitText("No edits yet")
	assertPreviewHeading(t, screen, "PREVIEW")
	fixture.capture("no-edits")
}

func TestIntegrationLiveOrderAndUpdates(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.appendOutput("codex", "LIVE_OUTPUT")
	fixture.waitText("LIVE_OUTPUT")
	// A newer timestamp updates status and moves the session into activity order.
	session := fixture.sessions["claude"]
	session.LastActiveAt = time.Now()
	fixture.sessions["claude"] = session
	fixture.setState("claude", agent.StateWaiting)
	screen := fixture.waitFor("live activity order", 4*time.Second, func(screen string) bool {
		codexRow, claudeRow := strings.Index(screen, "codex fixture"), strings.Index(screen, "claude fixture")
		return strings.Contains(screen, "1 needs input") && strings.Contains(screen, "|  LIVE") && claudeRow >= 0 && codexRow > claudeRow
	})
	if !strings.Contains(screen, "LIVE_OUTPUT") {
		t.Fatal("reordering sessions lost the selected preview")
	}
	fixture.capture("live-order-and-output")
	fixture.key("Tab")
	// Arrow sequences stay separate when the terminal batches rapid key presses.
	fixture.key("Down") // Active
	fixture.key("Down") // Recent
	fixture.key("Down") // Updates
	fixture.key("Enter")
	fixture.key("Tab")
	// Active also has one row; wait for the chosen filter before reading results.
	fixture.waitText("● Updates")
	screen = fixture.waitText("SESSIONS (1 of 1)")
	if !strings.Contains(screen, "claude fixture") || strings.Contains(screen, "codex fixture") || !strings.Contains(screen, "1 unseen") {
		t.Fatalf("Updates does not isolate the changed session:\n%s", screen)
	}
	fixture.capture("updates-filter")
	fixture.key("Enter")
	screen = fixture.waitText("No sessions match this filter")
	if !strings.Contains(screen, "0 unseen") {
		t.Fatal("acknowledgement did not update the header count")
	}
	fixture.capture("updates-acknowledged")
}

// Layout details live in the render tests; here we check the terminal shows
// the correct preview state alongside the selected-session heading.
func assertPreviewHeading(t *testing.T, screen, title string) {
	t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "SELECTED SESSION") && strings.Contains(line, title) {
			return
		}
	}
	t.Fatalf("preview heading %q is missing or on a different row:\n%s", title, screen)
}

func TestIntegrationNewAgentDialog(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("codex fixture")
	fixture.key("n")
	screen := fixture.waitText("NEW AGENT")
	if !strings.Contains(screen, "/fixture/codex") {
		t.Fatalf("wrong project:\n%s", screen)
	}
	fixture.capture("new-agent-codex")
	fixture.key("Right")
	fixture.capture("new-agent-claude")
	fixture.key("Enter")
	fixture.waitText("inside tmux")
	fixture.capture("new-agent-error")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.waitText("tab j/k s v q n/N")
	fixture.capture("new-agent-narrow")
	fixture.key("q")
	fixture.waitFor("dialog closed", 4*time.Second, func(screen string) bool { return !strings.Contains(screen, "NEW AGENT") })
	fixture.key("n")
	fixture.waitText("NEW AGENT")
	fixture.key("Escape")
}

func TestIntegrationNewAgentWindowDialog(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("codex fixture")
	fixture.key("N")
	fixture.waitText("Open in a new tmux window")
	fixture.capture("new-agent-window")
	fixture.key("q")
	fixture.waitFor("window dialog closed", 4*time.Second, func(screen string) bool { return !strings.Contains(screen, "NEW AGENT") })
	fixture.key("n")
	fixture.waitText("Open in a new tmux pane")
	fixture.key("Escape")
}
