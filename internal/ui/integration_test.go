//go:build integration

package ui_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"github.com/charmbracelet/lipgloss"
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
	assertDetailFramesAligned(t, screen)
	fixture.capture("idle-diff")
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
	assertDetailFramesAligned(t, screen)
	fixture.capture("wide-activity")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.waitText("v · Assistant: CLAUDE_ONLY")
	fixture.capture("narrow-activity")
	fixture.key("v")
	screen = fixture.waitText("LIVE ACTIVITY · Esc")
	if !strings.Contains(screen, "CLAUDE_ONLY") {
		t.Fatal("expanded feed lost the selected output")
	}
	fixture.capture("expanded-activity")
	fixture.key("Escape")
	fixture.waitText("SELECTED SESSION")
	fixture.selectProvider("codex")
	screen = fixture.waitText("CODEX_ONLY")
	if strings.Contains(screen, "CLAUDE_ONLY") {
		t.Fatal("returning to a cached session mixed providers")
	}
}

func TestIntegrationEmptyPreviewStates(t *testing.T) {
	fixture := newDashboardFixture(t)
	screen := fixture.waitText("Waiting for output")
	assertDetailFramesAligned(t, screen)
	fixture.capture("waiting-for-output")
	path := fixture.transcripts["codex"]
	if err := os.Rename(path, path+".hidden"); err != nil {
		t.Fatal(err)
	}
	screen = fixture.waitText("Preview unavailable")
	assertDetailFramesAligned(t, screen)
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
	assertDetailFramesAligned(t, screen)
	fixture.capture("no-edits")
}

func TestIntegrationHeldOrderAndUpdates(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.key("f")
	fixture.waitText("HELD")
	fixture.appendOutput("codex", "OUTPUT_WHILE_HELD")
	fixture.waitText("OUTPUT_WHILE_HELD")
	// A newer timestamp must update live status without moving existing rows.
	session := fixture.sessions["claude"]
	session.LastActiveAt = time.Now()
	fixture.sessions["claude"] = session
	fixture.setState("claude", agent.StateWaiting)
	screen := fixture.waitText("1 needs input")
	if strings.Index(screen, "codex fixture") > strings.Index(screen, "claude fixture") {
		t.Fatal("held rows reordered after a provider update")
	}
	fixture.capture("held-live-output")
	fixture.key("f")
	screen = fixture.waitFor("resumed activity order", 4*time.Second, func(screen string) bool {
		return strings.Contains(screen, "LIVE") && strings.Index(screen, "claude fixture") < strings.Index(screen, "codex fixture")
	})
	if !strings.Contains(screen, "OUTPUT_WHILE_HELD") {
		t.Fatal("resuming order lost the selected preview")
	}
	fixture.key("Tab")
	fixture.key("j") // Active
	fixture.key("j") // Recent
	fixture.key("j") // Updates
	fixture.key("Enter")
	fixture.key("Tab")
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

// Check the drawn boundaries, including empty previews that have no heading.
func assertDetailFramesAligned(t *testing.T, screen string) {
	t.Helper()
	top, bottom, heading, sessionRow := -1, -1, -1, -1
	for row, line := range strings.Split(screen, "\n") {
		if strings.Count(line, "╭") == 2 && strings.Count(line, "╮") == 2 {
			top = row
		}
		if strings.Count(line, "╰") == 2 && strings.Count(line, "╯") == 2 {
			bottom = row
		}
		if strings.Contains(line, "SELECTED SESSION") {
			heading = row
		}
		if strings.Contains(line, "Session ") {
			sessionRow = row
		}
	}
	if top < 0 || bottom <= top || heading != top+1 || sessionRow <= heading || sessionRow >= bottom {
		t.Fatalf("details and preview frames do not align or contain the metadata: top=%d bottom=%d heading=%d session=%d\n%s", top, bottom, heading, sessionRow, screen)
	}
	lines := strings.Split(screen, "\n")
	for _, corners := range [][2]string{{"╭", "╰"}, {"╮", "╯"}} {
		topLine, bottomLine := lines[top], lines[bottom]
		for range 2 {
			topIndex := strings.Index(topLine, corners[0])
			bottomIndex := strings.Index(bottomLine, corners[1])
			if lipgloss.Width(topLine[:topIndex]) != lipgloss.Width(bottomLine[:bottomIndex]) {
				t.Fatal("detail frame corners do not share the same columns")
			}
			topLine = topLine[topIndex+len(corners[0]):]
			bottomLine = bottomLine[bottomIndex+len(corners[1]):]
		}
	}
}
