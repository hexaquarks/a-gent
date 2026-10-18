//go:build integration

package ui_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"github.com/mattn/go-runewidth"
)

func TestIntegrationGraphLivePreviewAndResize(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.appendOutput("codex", "GRAPH_CODEX_FIRST")
	fixture.waitText("GRAPH_CODEX_FIRST")
	fixture.key("m")
	fixture.waitText("running · 2")
	fixture.key("m")
	fixture.waitText("2 projects ·")
	fixture.capture("graph-projects")
	fixture.width, fixture.height = 190, 50
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "190", "-y", "50")
	fixture.waitText("2 projects ·")
	fixture.key("Enter")
	fixture.waitText("SELECTED AGENT")
	fixture.capture("graph-project-agents")

	fixture.appendOutput("codex", "GRAPH_CODEX_SECOND")
	fixture.waitFor("graph output refresh", 4*time.Second, func(screen string) bool {
		return strings.Contains(screen, "GRAPH_CODEX_SECOND") && !strings.Contains(screen, "GRAPH_CODEX_FIRST")
	})
	fixture.capture("graph-live-feed")
	fixture.appendEdit("completed", "graph-verified.go")
	fixture.setState("codex", agent.StateIdle)
	fixture.waitText("+verified change")
	fixture.waitText("● idle")
	fixture.capture("graph-idle-edit")
	fixture.width, fixture.height = 190, 50
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "190", "-y", "50")
	fixture.waitText("SELECTED AGENT")
	fixture.capture("graph-wide-idle-edit")
	fixture.setState("codex", agent.StateError)
	fixture.waitText("● error")
	fixture.capture("graph-selected-error")
	fixture.setState("codex", agent.StateIdle)
	fixture.waitText("● idle")
	fixture.key("v")
	fixture.waitText("+verified change")
	fixture.key("Escape")
	fixture.waitText("SELECTED AGENT")

	fixture.appendOutput("claude", "GRAPH_CLAUDE_ONLY")
	fixture.key("Escape")
	fixture.waitText("2 projects ·")
	fixture.key("h")
	fixture.key("Enter")
	screen := fixture.waitText("GRAPH_CLAUDE_ONLY")
	if strings.Contains(screen, "GRAPH_CODEX_SECOND") || strings.Contains(screen, "graph-verified.go") {
		t.Fatal("graph selection leaked the previous preview")
	}
	fixture.capture("graph-selected-claude")

	fixture.width, fixture.height = 70, 24
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "70", "-y", "24")
	fixture.waitFor("narrow graph", 4*time.Second, func(screen string) bool {
		return len([]rune(strings.Split(screen, "\n")[0])) == 70 && strings.Contains(screen, "q/Esc projects")
	})
	fixture.capture("graph-narrow")
	fixture.key("Escape")
	fixture.key("]")
	fixture.key("Enter")
	fixture.key("v")
	fixture.waitText("graph-verified.go")
	fixture.key("Escape")
	fixture.key("m")
	fixture.waitText("ACTIVITY")
}

func TestIntegrationGraphNineAgentsAndOverflow(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.width, fixture.height = 190, 50
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "190", "-y", "50")
	fixture.key("m")
	fixture.waitText("running · 2")
	fixture.key("m")
	writeGraphSessions(t, fixture, 9, 4)
	fixture.waitText("9 sessions")
	fixture.waitText("5 projects ·")
	fixture.waitText("4 workers")
	fixture.capture("graph-nine-agents")
	fixture.waitText("5 projects ·")
	clickGraphText(t, fixture, "│ asteria")
	fixture.waitText("a-gent / asteria")
	fixture.waitText("2 agents ·")
	fixture.capture("graph-open-project")
	fixture.key("q")
	fixture.waitText("5 projects ·")
	fixture.key("j")
	fixture.waitText("5 projects · moonbeam ·")
	fixture.key("k")
	fixture.waitText("5 projects · asteria ·")
	fixture.capture("graph-spatial-up")
	writeGraphSessions(t, fixture, 15, 0)
	fixture.waitText("15 sessions")
	fixture.capture("graph-fifteen-agents")
	writeGraphSessions(t, fixture, 9, 0)
	fixture.waitText("9 sessions")

	fixture.width, fixture.height = 120, 29
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "120", "-y", "29")
	waitGraphResize(fixture)
	fixture.waitText("5 projects ·")
	fixture.capture("graph-default-size")

	fixture.width, fixture.height = 100, 28
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "100", "-y", "28")
	waitGraphResize(fixture)
	fixture.waitText("5 projects ·")
	fixture.capture("graph-nine-compact")
	writeGraphSessions(t, fixture, 100, 0)
	fixture.waitText("100 sessions")
	fixture.key("Enter")
	fixture.waitText("20 agents ·")
	fixture.capture("graph-large-project")
	for range 3 {
		fixture.key("[")
	}
	fixture.waitText("page 1/")
	fixture.key("]")
	fixture.waitText("page 2/")
	fixture.capture("graph-overflow-page-two")
	fixture.key("[")
	fixture.waitText("page 1/")
	fixture.key("q")
	fixture.waitText("5 projects ·")
}

func writeGraphSessions(t *testing.T, fixture *dashboardFixture, count, workers int) {
	t.Helper()
	projects := []string{"asteria", "copperline", "moonbeam", "paperboat", "sundial"}
	names := []string{
		"Fix sidebar focus", "Review retry behavior", "Keep live previews",
		"Add follow-up prompts", "Add deployment checks", "Show trip stops",
		"Polish project cards", "Track time per move", "Clarify retry behavior",
	}
	for _, provider := range []string{"codex", "claude"} {
		var sessions []agent.Session
		for index := 0; index < count; index++ {
			currentProvider := "codex"
			if index%2 != 0 {
				currentProvider = "claude"
			}
			if currentProvider != provider {
				continue
			}
			state := agent.StateRunning
			if index%3 == 1 {
				state = agent.StateWaiting
			}
			if index%3 == 2 {
				state = agent.StateIdle
			}
			sessions = append(sessions, agent.Session{
				ID: fmt.Sprintf("graph-%03d", index), Provider: provider, Name: names[index%len(names)],
				WorkingDirectory: "/fixture/" + projects[index%len(projects)], State: state,
			})
		}
		if provider == "codex" && workers > 0 {
			parent := &sessions[len(sessions)-1]
			parent.State = agent.StateRunning
			parentID, directory := parent.ID, parent.WorkingDirectory
			for index := range workers {
				sessions = append(sessions, agent.Session{
					ID: fmt.Sprintf("worker-%03d", index), Provider: provider, ParentID: parentID,
					Name: "Worker hidden inside parent", WorkingDirectory: directory, State: agent.StateIdle,
				})
			}
		}
		data, err := json.Marshal(sessions)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(fixture.root, provider+"-sessions.json")
		if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
	}
}

func waitGraphResize(fixture *dashboardFixture) {
	fixture.t.Helper()
	fixture.waitFor("graph resized without clipping", 4*time.Second, func(screen string) bool {
		first := strings.Split(screen, "\n")[0]
		return runewidth.StringWidth(first) == fixture.width && strings.HasSuffix(first, "╮")
	})
}

func TestIntegrationGraphShowsOnlyParentNodes(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.width, fixture.height = 190, 50
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "190", "-y", "50")
	parent := fixture.sessions["codex"]
	children := []agent.Session{
		parent,
		{ID: "child", Provider: "codex", ParentID: parent.ID,
			Name: "CHILD_NODE_HIDDEN", WorkingDirectory: parent.WorkingDirectory, State: agent.StateRunning},
		{ID: "nested", Provider: "codex", ParentID: "child",
			Name: "NESTED_NODE_HIDDEN", WorkingDirectory: parent.WorkingDirectory, State: agent.StateIdle},
	}
	data, err := json.Marshal(children)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.root, "codex-sessions.json")
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
	fixture.waitText("CHILD_NODE_HIDDEN")
	fixture.key("m")
	fixture.waitText("running ·")
	fixture.key("m")
	screen := fixture.waitText("2 projects ·")
	if strings.Contains(screen, "CHILD_NODE_HIDDEN") || strings.Contains(screen, "NESTED_NODE_HIDDEN") {
		t.Fatal("graph rendered a child node")
	}
	fixture.capture("graph-parent-projects")
	fixture.key("Enter")
	fixture.waitText("1 agent ·")
	fixture.capture("graph-parents-only")
	fixture.key("m")
	fixture.waitText("CHILD_NODE_HIDDEN")
}

// Send a real terminal mouse report through Bubble Tea's input decoder.
func clickGraphText(t *testing.T, fixture *dashboardFixture, label string) {
	t.Helper()
	screen := fixture.waitText(label)
	for row, line := range strings.Split(screen, "\n") {
		column := strings.Index(line, label)
		if column < 0 {
			continue
		}
		terminalColumn := runewidth.StringWidth(line[:column])
		report := fmt.Sprintf("\x1b[<0;%d;%dM", terminalColumn+1, row+1)
		fixture.tmux("send-keys", "-t", "dashboard:0.0", "-l", report)
		fixture.tmux("send-keys", "-t", "dashboard:0.0", "-l",
			fmt.Sprintf("\x1b[<0;%d;%dm", terminalColumn+1, row+1))
		return
	}
	t.Fatalf("graph card %q missing", label)
}

func TestIntegrationGraphUnevenProjectsAndRemoval(t *testing.T) {
	fixture := newDashboardFixture(t)
	fixture.width, fixture.height = 190, 50
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "190", "-y", "50")
	fixture.key("m")
	fixture.waitText("running · 2")
	fixture.key("m")

	projects := []string{"api", "ui", "docs", "workers", "作業", "large", "tools"}
	counts := []int{1, 2, 3, 7, 1, 25, 2}
	var sessions []agent.Session
	for project, count := range counts {
		for index := range count {
			provider := "codex"
			if index%2 != 0 {
				provider = "claude"
			}
			sessions = append(sessions, agent.Session{
				ID: fmt.Sprintf("project-%d-agent-%d", project, index), Provider: provider,
				Name:             fmt.Sprintf("作業 %d · %s", index, strings.Repeat("long title ", 8)),
				WorkingDirectory: "/fixture/" + projects[project], State: agent.StateRunning,
			})
		}
	}
	sessions = append(sessions, agent.Session{
		ID: "hidden-worker", Provider: "codex", ParentID: "project-5-agent-0",
		Name: "HIDDEN_WORKER_NODE", WorkingDirectory: "/fixture/large", State: agent.StateWaiting,
	})
	writeGraphFixtureSessions(t, fixture, sessions)
	fixture.waitText("41 sessions")
	fixture.waitText("26 agents")
	fixture.waitText("1 worker · 1 need input")
	screen := fixture.waitText("7 projects ·")
	if strings.Contains(screen, "HIDDEN_WORKER_NODE") {
		t.Fatal("overview exposed a worker as an agent row")
	}
	fixture.capture("graph-uneven-projects")
	clickGraphText(t, fixture, "│ large")
	fixture.waitText("25 agents ·")
	fixture.capture("graph-uneven-large-project")

	fixture.width, fixture.height = 100, 28
	fixture.tmux("resize-window", "-t", "dashboard:0", "-x", "100", "-y", "28")
	waitGraphResize(fixture)
	fixture.waitText("page 1/")
	fixture.key("]")
	fixture.waitText("page 2/")
	fixture.capture("graph-uneven-large-page-two")

	// Removing the open project must leave a usable empty view and a way back.
	var remaining []agent.Session
	for _, session := range sessions {
		if session.WorkingDirectory != "/fixture/large" {
			remaining = append(remaining, session)
		}
	}
	writeGraphFixtureSessions(t, fixture, remaining)
	fixture.waitText("No sessions match this filter.")
	fixture.capture("graph-removed-project")
	fixture.key("q")
	fixture.waitText("6 projects ·")
	fixture.capture("graph-after-project-removal")
	fixture.key("Enter")
	fixture.waitText("q/Esc projects")
	fixture.key("Escape")
	fixture.waitText("6 projects ·")
}

func writeGraphFixtureSessions(t *testing.T, fixture *dashboardFixture, sessions []agent.Session) {
	t.Helper()
	for _, provider := range []string{"codex", "claude"} {
		var records []agent.Session
		for _, session := range sessions {
			if session.Provider == provider {
				records = append(records, session)
			}
		}
		data, err := json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(fixture.root, provider+"-sessions.json")
		if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
	}
}
