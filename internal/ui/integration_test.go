//go:build integration

package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"github.com/mattn/go-runewidth"
)

func TestIntegrationBoardLayout(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("claude fixture")
	fixture.appendOutput("codex", "BOARD_OUTPUT")
	fixture.waitText("BOARD_OUTPUT")
	fixture.key("m")
	fixture.waitText("running · 2")
	fixture.capture("board-running")
	fixture.setState("codex", agent.StateWaiting)
	screen := fixture.waitText("needs you · 1")
	if !strings.Contains(screen, "BOARD_OUTPUT") || !strings.Contains(screen, "codex fixture") {
		t.Fatal("status change lost the selected card or preview")
	}
	fixture.capture("board-waiting")
	fixture.appendOutput("claude", "CLAUDE_OUTPUT")
	fixture.key("j")
	fixture.waitText("BOARD_OUTPUT")
	fixture.key("h")
	fixture.waitFor("focused Claude card", 4*time.Second, func(screen string) bool {
		for _, line := range strings.Split(screen, "\n") {
			if strings.Contains(line, "FOCUS") && strings.Contains(line, "claude fixture") {
				return true
			}
		}
		return false
	})
	screen = fixture.waitText("CLAUDE_OUTPUT")
	if strings.Contains(screen, "BOARD_OUTPUT") {
		t.Fatal("column navigation retained another session's output")
	}
	fixture.capture("board-selected-claude")
	fixture.key("v")
	fixture.waitText("LIVE ACTIVITY · Esc/q")
	fixture.key("h")
	fixture.waitText("CLAUDE_OUTPUT")
	fixture.capture("board-expanded-claude")
	fixture.key("Escape")
	fixture.waitText("FOCUS")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.waitText("running · 1")
	fixture.capture("board-narrow")
	fixture.key("Right")
	screen = fixture.waitText("BOARD_OUTPUT")
	if !strings.Contains(screen, "needs you · 1") || strings.Contains(screen, "CLAUDE_OUTPUT") {
		t.Fatal("narrow navigation did not follow the selected column and preview")
	}
	fixture.capture("board-narrow-codex")
	fixture.key("Left")
	fixture.waitText("CLAUDE_OUTPUT")
	fixture.capture("board-narrow-claude")
	fixture.key("m")
	fixture.waitFor("returned to list", 4*time.Second, func(screen string) bool {
		return strings.Contains(screen, "Age") && !strings.Contains(screen, "running ·")
	})
}

func TestIntegrationBoardCompactFocus(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("claude fixture")
	fixture.appendEdit("completed", "verified.go")
	fixture.setState("codex", agent.StateIdle)
	fixture.waitText("verified.go")
	fixture.key("m")
	screen := fixture.waitText("FOCUS")
	if !strings.Contains(screen, "verified.go") || !strings.Contains(screen, "+1 −1") ||
		strings.Contains(screen, "ACTIVITY") || strings.Contains(screen, "+verified change") {
		t.Fatalf("board did not compact the selected edit:\n%s", screen)
	}
	fixture.capture("compact-idle-edit")
	fixture.key("v")
	screen = fixture.waitText("EDIT PREVIEW · Esc/q")
	if !strings.Contains(screen, "+verified change") {
		t.Fatal("expanded preview lost the selected edit")
	}
	fixture.key("Escape")
	fixture.waitText("FOCUS")
	if err := os.Rename(fixture.transcripts["codex"], fixture.transcripts["codex"]+".hidden"); err != nil {
		t.Fatal(err)
	}
	screen = fixture.waitText("stale")
	if !strings.Contains(screen, "verified.go") {
		t.Fatal("compact focus discarded the stale cached edit")
	}
	fixture.capture("compact-stale-edit")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.waitFor("narrow compact focus", 4*time.Second, func(screen string) bool {
		return runewidth.StringWidth(strings.Split(screen, "\n")[0]) == 80 &&
			strings.Contains(screen, "FOCUS") && strings.Contains(screen, "stale") && strings.Contains(screen, "v details / diff")
	})
	fixture.capture("compact-narrow-edit")
	fixture.key("m")
	fixture.waitText("ACTIVITY")
}

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

func TestIntegrationNewAgentProviderLayout(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.key("n")
	fixture.waitText("NEW AGENT")
	for _, width := range []int{120, 80, 50} {
		fixture.width, fixture.height = width, 24
		fixture.tmux("resize-window", "-t", "dashboard:0", "-x", fmt.Sprint(width), "-y", "24")
		screen := fixture.waitFor("provider choices within dialog", 4*time.Second, func(screen string) bool {
			if runewidth.StringWidth(strings.Split(screen, "\n")[0]) != width {
				return false
			}
			for _, provider := range agent.InteractiveProviders() {
				if !strings.Contains(screen, provider) {
					return false
				}
			}
			for _, line := range strings.Split(screen, "\n") {
				if strings.Contains(line, "codex") && strings.Contains(line, "claude") {
					if strings.Contains(line, "│") && (width < 80 || strings.Contains(line, "hermes")) {
						return true
					}
				}
			}
			return false
		})
		for _, line := range strings.Split(screen, "\n") {
			if strings.Contains(line, "hermes") && !strings.Contains(line, "│") {
				t.Fatalf("provider escaped dialog border:\n%s", screen)
			}
		}
		fixture.capture(fmt.Sprintf("new-agent-%d", width))
	}
	fixture.key("Left")
	fixture.waitText("hermes")
	fixture.key("q")
	fixture.waitText("ACTIVITY")
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
	assertPreviewHeading(t, screen, "LATEST EDIT")
	fixture.capture("idle-diff")
	fixture.key("v")
	fixture.waitText("EDIT PREVIEW · Esc/q")
	fixture.key("q")
	fixture.waitText("ACTIVITY")
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
	if !strings.Contains(screen, "Claude is thinking") {
		t.Fatal("running Claude session has no thinking indicator")
	}
	if strings.Contains(screen, "LIVE ACTIVITY · v") || !strings.Contains(screen, "v expand") {
		t.Fatal("live activity shortcut is not at the bottom of the preview")
	}
	if strings.Contains(screen, "CODEX_ONLY") {
		t.Fatal("session switch retained another provider's output")
	}
	assertPreviewHeading(t, screen, "ACTIVITY")
	assertProjectColumn(t, screen, true)
	fixture.capture("wide-activity")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	screen = fixture.waitFor("narrow activity inspector", 4*time.Second, func(screen string) bool {
		for _, line := range strings.Split(screen, "\n") {
			if strings.Contains(line, "Agent") && strings.Contains(line, "Age") {
				return !strings.Contains(line, "Project") && strings.Contains(screen, "CLAUDE_ONLY")
			}
		}
		return false
	})
	assertProjectColumn(t, screen, false)
	fixture.capture("narrow-activity")
	fixture.key("v")
	screen = fixture.waitText("LIVE ACTIVITY · Esc/q")
	if !strings.Contains(screen, "CLAUDE_ONLY") {
		t.Fatal("expanded feed lost the selected output")
	}
	fixture.capture("expanded-activity")
	fixture.key("q")
	fixture.waitText("ACTIVITY")
	fixture.selectProvider("codex")
	screen = fixture.waitText("CODEX_ONLY")
	if strings.Contains(screen, "CLAUDE_ONLY") {
		t.Fatal("returning to a cached session mixed providers")
	}
}

func assertProjectColumn(t *testing.T, screen string, visible bool) {
	t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "Agent") && strings.Contains(line, "Age") {
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
	if !strings.Contains(screen, "Codex is thinking") {
		t.Fatal("running Codex session has no thinking indicator")
	}
	assertPreviewHeading(t, screen, "ACTIVITY")
	fixture.capture("waiting-for-output")
	path := fixture.transcripts["codex"]
	if err := os.Rename(path, path+".hidden"); err != nil {
		t.Fatal(err)
	}
	screen = fixture.waitText("Preview unavailable")
	assertPreviewHeading(t, screen, "ACTIVITY")
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
	if strings.Contains(screen, "Codex is thinking") {
		t.Fatal("idle session kept thinking indicator")
	}
	assertPreviewHeading(t, screen, "ACTIVITY")
	fixture.capture("no-edits")
}

func TestIntegrationLiveOrderAndRecent(t *testing.T) {
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
	fixture.key("Down") // Recent
	fixture.key("Enter")
	fixture.key("Tab")
	fixture.waitText("◷  Recent")
	screen = fixture.waitFor("Recent stopped session", 4*time.Second, func(screen string) bool {
		return strings.Contains(screen, "SESSIONS (1 of 1)") && strings.Contains(screen, "claude fixture") &&
			!strings.Contains(screen, "codex fixture") && strings.Contains(screen, "1 unseen")
	})
	fixture.capture("recent-filter")
	fixture.key("Enter")
	screen = fixture.waitText("0 unseen")
	if !strings.Contains(screen, "claude fixture") {
		t.Fatal("acknowledgement removed the session from Recent")
	}
	fixture.capture("recent-acknowledged")
}

// Layout details live in the render tests; the terminal must show both panel headings.
func assertPreviewHeading(t *testing.T, screen, title string) {
	t.Helper()
	if !strings.Contains(screen, title) || !strings.Contains(screen, "ACTIVITY") ||
		!strings.Contains(screen, "LATEST EDIT") {
		t.Fatalf("inspector panels or heading %q missing:\n%s", title, screen)
	}
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
	fixture.waitText("help")
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

func TestIntegrationReadyPulse(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.setState("codex", agent.StateIdle)
	fixture.waitText("1 unseen")
	// tmux versions emit RGB or indexed colors, with semicolon or colon separators.
	backgroundCodes := regexp.MustCompile(`48[;:](?:2[;:]{1,2}\d+[;:]\d+[;:]\d+|5[;:]\d+)`)
	row := func() string {
		screen := fixture.tmux("capture-pane", "-p", "-e", "-t", "dashboard:0.0")
		for _, line := range strings.Split(screen, "\n") {
			if strings.Contains(line, "codex fixture") && strings.Contains(line, "idle") {
				// Ignore the changing Age timestamp; inspect only backgrounds.
				colors := backgroundCodes.FindAllString(line, -1)
				if len(colors) == 0 {
					t.Fatalf("idle session has no background color: %q", line)
				}
				return strings.Join(colors, " ")
			}
		}
		t.Fatal("idle session row missing")
		return ""
	}
	initial := row()
	fixture.waitFor("animated row background", time.Second, func(string) bool { return row() != initial })
	// Capture near the crest of the pulse for visual review.
	crest := time.Now().Add(500 * time.Millisecond)
	fixture.waitFor("pulse crest", 2*time.Second, func(string) bool { return time.Now().After(crest) })
	fixture.capture("ready-pulse")
	glowing := row()
	settled := time.Now().Add(1600 * time.Millisecond)
	fixture.waitFor("pulse settled", 3*time.Second, func(string) bool { return time.Now().After(settled) })
	normal := row()
	if normal == glowing {
		t.Fatal("row background did not fade")
	}
	fixture.capture("ready-settled")
	stable := time.Now().Add(300 * time.Millisecond)
	fixture.waitFor("settled background remains stable", time.Second, func(string) bool {
		if row() != normal {
			t.Fatal("settled row continued animating")
		}
		return time.Now().After(stable)
	})
}

func TestIntegrationKeyboardHelp(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.key("?")
	fixture.waitText("KEYBOARD HELP")
	fixture.capture("help-top")
	fixture.key("End")
	fixture.waitText("Quit dashboard")
	fixture.capture("help-bottom")
	fixture.key("q")
	fixture.waitFor("help closed", time.Second, func(screen string) bool { return !strings.Contains(screen, "KEYBOARD HELP") })
	fixture.capture("compact-footer")
	fixture.key("v")
	fixture.waitText("Esc/q")
	fixture.key("?")
	fixture.waitText("KEYBOARD HELP")
	fixture.key("Escape")
	fixture.waitFor("help returned to preview", time.Second, func(screen string) bool {
		return !strings.Contains(screen, "KEYBOARD HELP") && strings.Contains(screen, "Esc/q")
	})
	fixture.key("q")
	fixture.waitText("ACTIVITY")
	fixture.key("?")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.waitText("KEYBOARD HELP")
	fixture.capture("help-narrow")
	fixture.width, fixture.height = 40, 16
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "40", "-y", "16")
	// Wait for the rendered frame, not text that was already visible before resizing.
	fixture.waitText("╭" + strings.Repeat("─", fixture.width-2) + "╮")
	fixture.waitText("↑/↓ scroll")
	fixture.capture("help-small")
	fixture.key("End")
	fixture.waitText("Quit dashboard")
	fixture.key("q")
	fixture.waitFor("narrow help closed", time.Second, func(screen string) bool { return !strings.Contains(screen, "KEYBOARD HELP") })
}

func TestIntegrationBoardCardStyling(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("claude fixture")
	fixture.waitText("codex fixture")
	fixture.key("m")
	fixture.waitText("running · 2")
	screen := fixture.tmux("capture-pane", "-p", "-e", "-t", "dashboard:0.0")
	if !regexp.MustCompile(`38;2;89;223;240m(?:\x1b\[[0-9;]*m)*┌`).MatchString(screen) ||
		!strings.Contains(screen, "▌") || !strings.Contains(screen, "38;2;163;239;120") {
		t.Fatalf("board lost its selected cyan frame or colored running edges:\n%s", screen)
	}
	plain := fixture.tmux("capture-pane", "-p", "-t", "dashboard:0.0")
	if strings.Contains(plain, "›") {
		t.Fatalf("board kept its redundant selection arrow:\n%s", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "running · 2") && strings.Count(line, "│") < 4 {
			t.Fatalf("board lost vertical column dividers: %s", line)
		}
	}
	if !strings.Contains(plain, "────────────────") || !strings.Contains(plain, "FOCUS") ||
		!strings.Contains(plain, "v details / diff") {
		t.Fatalf("board lost heading underlines, compact selected session, or footer:\n%s", plain)
	}
	fixture.capture("styled-board")

	cardPositions := func(screen string) map[string]int {
		positions := make(map[string]int)
		for row, line := range strings.Split(screen, "\n") {
			if strings.Contains(line, "FOCUS") {
				break
			}
			for _, name := range []string{"codex fixture", "claude fixture"} {
				if strings.Contains(line, name) {
					positions[name] = row
				}
			}
		}
		return positions
	}
	before := cardPositions(plain)
	movements := []struct {
		key  string
		name string
	}{
		{"j", "claude fixture"},
		{"k", "codex fixture"},
	}

	for _, movement := range movements {
		fixture.key(movement.key)
		screen := fixture.waitFor("selected card", 4*time.Second, func(screen string) bool {
			for _, line := range strings.Split(screen, "\n") {
				if strings.Contains(line, "FOCUS") && strings.Contains(line, movement.name) {
					return true
				}
			}
			return false
		})
		after := cardPositions(screen)
		for _, name := range []string{"codex fixture", "claude fixture"} {
			beforeRow, beforeVisible := before[name]
			afterRow, afterVisible := after[name]
			if !beforeVisible || !afterVisible || beforeRow != afterRow {
				t.Fatalf("selection moved card %s from row %d to %d", name, beforeRow, afterRow)
			}
		}
	}
}

func TestIntegrationDashboardRemembersLayout(t *testing.T) {
	fixture := newDashboardFixture(t)
	for _, board := range []bool{true, false} {
		fixture.key("m")
		if board {
			fixture.waitText("running · 2")
		} else {
			fixture.waitText("ACTIVITY")
		}

		fixture.key("q")
		exitPath := filepath.Join(fixture.root, "exit-status")
		fixture.waitFor("dashboard exit", 5*time.Second, func(_ string) bool {
			status, err := os.ReadFile(exitPath)
			return err == nil && string(status) == "0"
		})
		if err := os.Remove(exitPath); err != nil {
			t.Fatal(err)
		}
		fixture.tmux("respawn-pane", "-t", "dashboard:0.0", fixture.launchCommand)
		fixture.waitText("2 sessions")
		if board {
			fixture.waitText("running · 2")
			fixture.capture("reopened-board")
		} else {
			fixture.waitText("ACTIVITY")
			fixture.capture("reopened-list")
		}
	}
}

func TestIntegrationBoardActivityAndObservedCompletion(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("codex fixture")
	fixture.waitText("claude fixture")
	fixture.appendOutput("codex", "CARD_ONE")
	fixture.waitText("CARD_ONE")
	fixture.key("m")
	fixture.waitText("FOCUS")

	// The compact inspector also contains output. Require it in the cards.
	cardArea := func(screen string) string {
		return strings.Split(screen, "FOCUS")[0]
	}
	waitCard := func(text string) string {
		t.Helper()
		return fixture.waitFor("card summary "+text, 4*time.Second, func(screen string) bool {
			return strings.Contains(cardArea(screen), text)
		})
	}
	waitCard("CARD_ONE")
	fixture.capture("card-first-output")
	fixture.appendOutput("codex", "CARD_TWO")
	waitCard("CARD_TWO")
	fixture.capture("card-updated-output")

	fixture.selectProvider("claude")
	fixture.appendOutput("claude", "CLAUDE_CARD")
	screen := waitCard("CLAUDE_CARD")
	if strings.Contains(cardArea(screen), "CARD_TWO") {
		t.Fatalf("unselected card retained current-output summary:\n%s", screen)
	}
	fixture.capture("card-selected-claude")

	fixture.setState("codex", agent.StateIdle)
	waitCard("Finished this visit")
	fixture.capture("card-finished-this-visit")
	fixture.setState("codex", agent.StateRunning)
	fixture.waitFor("rerun clears finished hint", 4*time.Second, func(screen string) bool {
		return strings.Contains(screen, "running · 2") &&
			!strings.Contains(cardArea(screen), "Finished this visit")
	})

	fixture.setState("claude", agent.StateWaiting)
	waitCard("Waiting for input")
	fixture.width, fixture.height = 64, 20
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "64", "-y", "20")
	screen = waitCard("Waiting for input")
	if runewidth.StringWidth(strings.Split(screen, "\n")[0]) != fixture.width {
		t.Fatal("narrow board exceeded the terminal width")
	}
	fixture.capture("card-narrow-waiting")
}

func TestIntegrationInspectorActivityDiffAndCollapse(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.waitText("claude fixture")
	fixture.appendEdit("completed", "together.go")
	fixture.appendOutput("codex", "TESTING_TOGETHER")
	fixture.waitFor("activity alongside completed edit", 4*time.Second, func(screen string) bool {
		return strings.Contains(screen, "TESTING_TOGETHER") && strings.Contains(screen, "+verified change") &&
			strings.Contains(screen, "together.go")
	})
	fixture.capture("activity-and-diff")
	fixture.key("i")
	fixture.waitFor("inspector collapsed", time.Second, func(screen string) bool {
		return strings.Contains(screen, "codex fixture") && !strings.Contains(screen, "LATEST EDIT")
	})
	fixture.capture("inspector-collapsed")
	fixture.key("v")
	fixture.waitText("LIVE ACTIVITY · Esc/q")
	fixture.key("Escape")
	fixture.waitFor("expanded preview returned to collapsed inspector", time.Second, func(screen string) bool {
		return strings.Contains(screen, "codex fixture") && !strings.Contains(screen, "LATEST EDIT") &&
			!strings.Contains(screen, "Esc/q")
	})
	fixture.key("i")
	fixture.waitText("TESTING_TOGETHER")
	fixture.waitText("together.go")
}

func TestIntegrationBoardInspectorKeepsActivityAndLatestEdit(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.appendEdit("completed", "board.go")
	fixture.appendOutput("codex", "BOARD_TESTING")
	fixture.waitText("BOARD_TESTING")
	fixture.waitText("board.go")
	fixture.key("m")
	fixture.waitFor("compact board activity and edit", 4*time.Second, func(screen string) bool {
		parts := strings.SplitN(screen, "FOCUS", 2)
		return len(parts) == 2 && strings.Contains(parts[1], "BOARD_TESTING") &&
			strings.Contains(parts[1], "board.go") && strings.Contains(parts[1], "+1 −1")
	})
	fixture.capture("compact-activity-and-edit")
	fixture.width, fixture.height = 160, 40
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "160", "-y", "40")
	fixture.waitFor("roomy board inspector", 4*time.Second, func(screen string) bool {
		lines := strings.Split(screen, "\n")
		for index, line := range lines {
			if strings.Contains(line, "FOCUS") && index+2 < len(lines) {
				return strings.Contains(lines[index+1], "codex · codex") &&
					strings.Contains(lines[index+2], "BOARD_TESTING") && strings.Contains(line, "board.go")
			}
		}
		return false
	})
	fixture.capture("roomy-activity-and-edit")
	fixture.width, fixture.height = 80, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "80", "-y", "24")
	fixture.waitFor("stacked board inspector", 4*time.Second, func(screen string) bool {
		lines := strings.Split(screen, "\n")
		for index, line := range lines {
			if strings.Contains(line, "FOCUS") && index+2 < len(lines) {
				return strings.Contains(line, "v details / diff") &&
					strings.Contains(lines[index+1], "BOARD_TESTING") && strings.Contains(lines[index+2], "board.go")
			}
		}
		return false
	})
	fixture.capture("narrow-activity-and-edit")
	fixture.key("v")
	fixture.waitText("LIVE ACTIVITY · Esc/q")
	fixture.waitText("Tab switch")
	fixture.key("Tab")
	fixture.waitText("EDIT PREVIEW · Esc/q")
	fixture.waitText("+verified change")
	fixture.capture("expanded-latest-edit")
	fixture.key("Tab")
	fixture.waitText("LIVE ACTIVITY · Esc/q")
	fixture.waitText("BOARD_TESTING")
	fixture.key("Escape")
	fixture.waitText("FOCUS")
}
