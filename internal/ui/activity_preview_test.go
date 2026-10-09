package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/codex"
	"a-gent/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestActivitySwitchesToRetainedDiffWhenIdle(t *testing.T) {
	model := NewModel(nil, WithPreviewSources(codex.NewAdapter()))
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one", State: agent.StateRunning}}})
	key := sessionIdentity{provider: "codex", id: "one"}
	model.previewKey = key
	model.previewCache = map[sessionIdentity]previewEntry{key: {loaded: true, checked: time.Now(), edit: &agent.Edit{Filename: "kept.go", Diff: "+kept"}, activity: &agent.Activity{Label: "Assistant", Text: "First public output", At: time.Now()}}}
	if view := model.previewView(40, 8, false); !strings.Contains(view, "First public output") || strings.Contains(view, "kept.go") {
		t.Fatal(view)
	}
	updated, _ := model.Update(previewResult{key: key, edit: model.previewCache[key].edit, activity: &agent.Activity{Label: "Assistant", Text: "Second public output", At: time.Now()}})
	model = updated.(Model)
	if view := model.previewView(40, 8, false); !strings.Contains(view, "Second public output") || strings.Contains(view, "First public output") {
		t.Fatal(view)
	}
	updated, _ = model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one", State: agent.StateIdle}}})
	model = updated.(Model)
	if view := model.previewView(40, 8, false); !strings.Contains(view, "kept.go") {
		t.Fatal(view)
	}
}

func TestActivityLayoutAndRightEdge(t *testing.T) {
	for _, width := range []int{64, 80, 100, 120, 180} {
		model := NewModel(nil, WithPreviewSources(codex.NewAdapter()))
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		model = updated.(Model)
		model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one", Name: "Active session", State: agent.StateRunning}}})
		model.previewCache = map[sessionIdentity]previewEntry{{provider: "codex", id: "one"}: {loaded: true, checked: time.Now(), activity: &agent.Activity{Label: "Assistant", Text: strings.Repeat("界", 200) + "\x1b[2J", At: time.Now()}}}
		screen := ansi.Strip(model.View())
		if lipgloss.Width(screen) > width || lipgloss.Height(screen) > 30 {
			t.Fatal("activity overflowed dashboard")
		}
		if width >= 100 {
			separatorEnd, boxEnd := -1, -1
			for _, line := range strings.Split(screen, "\n") {
				if strings.Contains(line, "│─") {
					separatorEnd = lipgloss.Width(strings.TrimRight(line, " "))
				}
				if strings.Contains(line, "╮") {
					boxEnd = lipgloss.Width(strings.TrimRight(line, " "))
				}
			}
			if boxEnd < 0 || boxEnd != separatorEnd {
				t.Fatalf("width %d: box=%d separator=%d", width, boxEnd, separatorEnd)
			}
		}
	}
}

func TestActivityExpandShortcutUsesFooter(t *testing.T) {
	model := NewModel(nil, WithPreviewSources(codex.NewAdapter()))
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one", State: agent.StateRunning}}})
	key := sessionIdentity{provider: "codex", id: "one"}
	model.previewCache = map[sessionIdentity]previewEntry{key: {
		loaded: true, checked: time.Now(),
		activity: &agent.Activity{Label: "Assistant", Text: "Latest output", At: time.Now()},
	}}

	view := ansi.Strip(model.previewView(40, 8, false))
	lines := strings.Split(view, "\n")
	if strings.TrimSpace(lines[0]) != "LIVE ACTIVITY" || !strings.Contains(lines[len(lines)-1], "Codex is thinking") ||
		!strings.Contains(lines[len(lines)-1], "v expand") || strings.Count(view, "v expand") != 1 {
		t.Fatal(view)
	}

	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one", State: agent.StateIdle}}})
	view = ansi.Strip(model.previewView(40, 8, false))
	lines = strings.Split(view, "\n")
	if !strings.Contains(lines[len(lines)-1], "v expand") || strings.Contains(view, "is thinking") {
		t.Fatal(view)
	}
}
