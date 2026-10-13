package ui

import (
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSubagentRowsStayWithParentsAndRemainSelectable(t *testing.T) {
	sessions := []agent.Session{
		{Provider: "codex", ID: "nested", ParentID: "child", Name: "A nested", State: agent.StateIdle},
		{Provider: "codex", ID: "child", ParentID: "parent", Name: "B child", State: agent.StateRunning},
		{Provider: "codex", ID: "other", Name: "C other", State: agent.StateIdle},
		{Provider: "codex", ID: "parent", Name: "D parent", State: agent.StateIdle},
	}
	model := NewModel(nil)
	model.sort = sessionSort{column: "Session"}
	updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: sessions})
	model = updated.(Model)
	ordered := model.filteredSessions()
	for index, id := range []string{"other", "parent", "child", "nested"} {
		if ordered[index].ID != id {
			t.Fatalf("row %d = %s, want %s", index, ordered[index].ID, id)
		}
	}
	if name := model.listSessionName(ordered[3]); name != "    ↳ [subagent] A nested" {
		t.Fatalf("nested name = %q", name)
	}
	model.table.SetCursor(2)
	selected, ok := model.selectedSession()
	if !ok || selected.ID != "child" {
		t.Fatalf("selected = %+v", selected)
	}
	row := ansi.Strip(model.sessionRowView(selected, true, model.table.Columns()))
	if !strings.Contains(row, "↳ [subagent] B child") {
		t.Fatalf("child marker absent: %s", row)
	}
	// Polling must preserve child selection even when the parent sort changes.
	sessions[3].Name = "A parent"
	updated, _ = model.Update(polling.Update{Provider: "codex", Sessions: sessions})
	model = updated.(Model)
	selected, _ = model.selectedSession()
	if selected.ID != "child" {
		t.Fatalf("poll changed selection to %s", selected.ID)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	model = updated.(Model)
	for _, session := range model.filteredSessions() {
		if session.ParentID != "" {
			t.Fatal("Board includes child rows")
		}
	}
}

func TestSubagentGroupingHandlesFiltersProvidersAndCycles(t *testing.T) {
	sessions := []agent.Session{
		{Provider: "codex", ID: "child", ParentID: "parent", Name: "Child", State: agent.StateRunning},
		{Provider: "claude", ID: "parent", Name: "Other provider", State: agent.StateIdle},
		{Provider: "codex", ID: "parent", Name: "Parent", State: agent.StateIdle},
	}
	ordered := groupSubagents(sessions)
	if ordered[0].Provider != "claude" || ordered[1].ID != "parent" || ordered[2].ID != "child" {
		t.Fatalf("provider grouping = %+v", ordered)
	}
	model := NewModel(nil)
	model.sessions = sessions
	model.selectedView = activeView
	filtered := model.filteredSessions()
	if len(filtered) != 1 || filtered[0].ID != "child" || model.listSessionName(filtered[0]) != "  ↳ [subagent] Child" {
		t.Fatalf("filtered child = %+v", filtered)
	}
	cyclic := []agent.Session{{Provider: "codex", ID: "a", ParentID: "b"}, {Provider: "codex", ID: "b", ParentID: "a"}}
	if len(groupSubagents(cyclic)) != 2 {
		t.Fatal("cycle hid rows")
	}
}

func TestSubagentsOpenTheParentWorkspace(t *testing.T) {
	navigator := &fakeNavigator{}
	model := NewModel(nil, WithSessionNavigator(navigator))
	model.sessions = []agent.Session{
		{Provider: "codex", ID: "parent", WorkingDirectory: "/parent"},
		{Provider: "codex", ID: "child", ParentID: "parent", WorkingDirectory: "/child"},
	}
	model.updateTableRows()
	model.table.SetCursor(1)
	_, command := model.navigateSelectedSession()
	command()
	if navigator.session.ID != "parent" || navigator.session.WorkingDirectory != "/parent" {
		t.Fatalf("opened %+v instead of parent", navigator.session)
	}
}

func TestNarrowListStillMarksSubagents(t *testing.T) {
	model := NewModel(nil)
	child := agent.Session{Provider: "codex", ID: "child", ParentID: "parent"}
	row := ansi.Strip(model.sessionRowView(child, false, tableColumns(40, "codex")))
	if !strings.Contains(row, "↳codex") {
		t.Fatalf("narrow child marker absent: %s", row)
	}
}
