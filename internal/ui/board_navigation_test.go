package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func navigationBoard() Model {
	model := NewModel(nil)
	model.boardVisible = true
	model.sort = sessionSort{column: "Session"}
	model.sessions = []agent.Session{
		{ID: "a0", Name: "A0", Provider: "codex", State: agent.StateWaiting},
		{ID: "a1", Name: "A1", Provider: "codex", State: agent.StateWaiting},
		{ID: "a2", Name: "A2", Provider: "codex", State: agent.StateWaiting},
		{ID: "b0", Name: "B0", Provider: "codex", State: agent.StateRunning},
		{ID: "b1", Name: "B1", Provider: "codex", State: agent.StateRunning},
		{ID: "c0", Name: "C0", Provider: "codex", State: agent.StateIdle},
	}
	model.updateTableRows()
	return model
}

func requireBoardSelection(t *testing.T, model Model, id string) {
	t.Helper()
	selected, ok := model.selectedSession()
	if !ok || selected.ID != id {
		t.Fatalf("selected %+v, expected %s", selected, id)
	}
}

func TestBoardNavigationClampsCardRowAndColumnBoundaries(t *testing.T) {
	arrowKeys := map[string]tea.KeyType{
		"j": tea.KeyDown,
		"k": tea.KeyUp,
		"h": tea.KeyLeft,
		"l": tea.KeyRight,
	}

	for _, arrows := range []bool{false, true} {
		model := navigationBoard()
		model.table.SetCursor(2)
		steps := []struct {
			key string
			id  string
		}{
			{"j", "a2"},
			{"l", "a2"},
			{"h", "c0"},
			{"h", "b0"},
			{"h", "b0"},
			{"j", "b1"},
			{"j", "b1"},
			{"l", "c0"},
			{"k", "c0"},
			{"l", "a0"},
			{"k", "a0"},
			{"j", "a1"},
			{"h", "c0"},
			{"l", "a0"},
		}
		for _, step := range steps {
			key := runeKey(rune(step.key[0]))
			if arrows {
				key.Type = arrowKeys[step.key]
			}
			model = sendProjectKey(model, key)
			requireBoardSelection(t, model, step.id)
		}
	}
}

func TestBoardNavigationSkipsEmptyColumns(t *testing.T) {
	model := navigationBoard()
	model.sessions = append(model.sessions[:3], model.sessions[5:]...)
	model.updateTableRows()
	model.table.SetCursor(2)
	model = sendProjectKey(model, runeKey('h'))
	requireBoardSelection(t, model, "c0")
	model = sendProjectKey(model, runeKey('l'))
	requireBoardSelection(t, model, "a0")

	model.sessions = model.sessions[:3]
	model.updateTableRows()
	model = sendProjectKey(model, runeKey('l'))
	requireBoardSelection(t, model, "a0")
	model.sessions = nil
	model.updateTableRows()
	for _, key := range "hljk" {
		model = sendProjectKey(model, runeKey(key))
	}
	if _, ok := model.selectedSession(); ok {
		t.Fatal("empty board selected a session")
	}
}

func TestBoardNavigationUsesFilteredSortAndPreservesLiveIdentity(t *testing.T) {
	model := navigationBoard()
	model.sessions = append(model.sessions, agent.Session{ID: "hidden", Provider: "claude", State: agent.StateRunning})
	model.selectedProvider = "codex"
	model.sort.descending = true
	model.updateTableRows()
	model.table.SetCursor(3) // A2 is first in Needs You under the reversed sort.
	requireBoardSelection(t, model, "a2")
	model = sendProjectKey(model, runeKey('h'))
	requireBoardSelection(t, model, "c0")
	model = sendProjectKey(model, runeKey('h'))
	requireBoardSelection(t, model, "b1")

	sessions := append([]agent.Session(nil), model.sessions[:6]...)
	sessions[4].State = agent.StateIdle
	updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: sessions})
	model = updated.(Model)
	requireBoardSelection(t, model, "b1")
	model = sendProjectKey(model, runeKey('h'))
	requireBoardSelection(t, model, "b0")
}

func TestBoardNavigationRespectsOtherKeyOwners(t *testing.T) {
	for _, owner := range []string{"list", "sidebar", "search", "help", "preview"} {
		t.Run(owner, func(t *testing.T) {
			model := navigationBoard()
			switch owner {
			case "list":
				model.boardVisible = false
			case "sidebar":
				model.sidebarFocus = true
			case "search":
				model.projectSearching = true
			case "help":
				model.helpOpen = true
			case "preview":
				model.previewExpanded = true
			}
			for _, key := range []tea.KeyMsg{runeKey('h'), runeKey('l'), {Type: tea.KeyLeft}, {Type: tea.KeyRight}} {
				model = sendProjectKey(model, key)
				requireBoardSelection(t, model, "a0")
			}
			if owner == "search" && model.projectQuery != "hl" {
				t.Fatalf("search did not receive column keys: %q", model.projectQuery)
			}
			if owner == "list" {
				model = sendProjectKey(model, runeKey('j'))
				requireBoardSelection(t, model, "a1")
			}
		})
	}
}

func TestBoardColumnNavigationPreviewIsolation(t *testing.T) {
	model := navigationBoard()
	model.previewCache = make(map[sessionIdentity]previewEntry)
	for _, session := range model.sessions {
		model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}] = previewEntry{
			loaded: true, checked: time.Now(),
			activity: &agent.Activity{Text: "OUTPUT_" + session.ID, At: time.Now()},
		}
	}
	model = sendProjectKey(model, runeKey('h'))
	requireBoardSelection(t, model, "c0")
	model = sendProjectKey(model, runeKey('h'))
	requireBoardSelection(t, model, "b0")
	view := ansi.Strip(model.previewView(50, 10, false))
	if !strings.Contains(view, "OUTPUT_b0") || strings.Contains(view, "OUTPUT_a0") {
		t.Fatalf("column change leaked another session's preview:\n%s", view)
	}
}

func TestBoardFooterKeepsEssentialActions(t *testing.T) {
	model := navigationBoard()
	model.navigator = &fakeNavigator{}
	for _, width := range []int{120, 80, 24} {
		model.width = width
		footer := ansi.Strip(model.footerText())
		for _, key := range []string{"tab", "q", "?"} {
			if !strings.Contains(footer, key) {
				t.Fatalf("width %d omitted %q: %s", width, key, footer)
			}
		}
		if !strings.Contains(footer, "enter") && !strings.Contains(footer, "↵") {
			t.Fatal("footer omitted opening shortcut")
		}
		if strings.Contains(footer, "h/l") || strings.Contains(footer, "j/k") {
			t.Fatalf("width %d retained board movement hints: %s", width, footer)
		}
	}
	help := ansi.Strip(strings.Join(helpLines(48), "\n"))
	if !strings.Contains(help, "Board column") || !strings.Contains(help, "Board card") {
		t.Fatal("help omitted board navigation")
	}
}
