package ui

import (
	"fmt"
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	header := ansi.Strip(model.headerView(summarizeSessions(model.sessions), 120))
	if !strings.Contains(header, "2 sessions · 2 subagents") {
		t.Fatalf("header did not distinguish sessions from subagents: %s", header)
	}
	ordered := model.filteredSessions()
	for index, id := range []string{"other", "parent", "child", "nested"} {
		if ordered[index].ID != id {
			t.Fatalf("row %d = %s, want %s", index, ordered[index].ID, id)
		}
	}
	if name := model.listSessionName(ordered[3]); name != "   └─ A nested" {
		t.Fatalf("nested name = %q", name)
	}
	model.table.SetCursor(2)
	selected, ok := model.selectedSession()
	if !ok || selected.ID != "child" {
		t.Fatalf("selected = %+v", selected)
	}
	row := ansi.Strip(model.sessionRowView(selected, true, model.table.Columns()))
	if !strings.Contains(row, "└─ B child") || strings.Contains(row, "[subagent]") {
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
	selected, _ = model.selectedSession()
	if selected.ID != "parent" {
		t.Fatalf("Board should select the child's parent, got %s", selected.ID)
	}
	model.boardView()
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	model = updated.(Model)
	selected, _ = model.selectedSession()
	if selected.ID != "parent" || len(model.filteredSessions()) != len(sessions) {
		t.Fatal("returning to List lost parent selection or subagents")
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
	if len(filtered) != 1 || filtered[0].ID != "child" || model.listSessionName(filtered[0]) != "└─ Child" {
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

func TestSessionTreeKeepsBranchesInTaskColumn(t *testing.T) {
	model := NewModel(nil)
	model.sort = sessionSort{column: "Session"}
	model.sessions = []agent.Session{
		{Provider: "codex", ID: "parent", Name: "Parent"},
		{Provider: "codex", ID: "a", ParentID: "parent", Name: "A worker"},
		{Provider: "codex", ID: "nested", ParentID: "a", Name: "Nested task"},
		{Provider: "codex", ID: "b", ParentID: "parent", Name: "B worker"},
		{Provider: "claude", ID: "foreign", ParentID: "parent", Name: "Other provider"},
	}
	want := map[string]string{
		"parent":  "Parent",
		"a":       "├─ A worker",
		"nested":  "│  └─ Nested task",
		"b":       "└─ B worker",
		"foreign": "└─ Other provider",
	}
	for _, session := range model.sessions {
		if got := model.listSessionName(session); got != want[session.ID] {
			t.Fatalf("session %s = %q, want %q", session.ID, got, want[session.ID])
		}
		row := ansi.Strip(model.sessionRowView(session, false, tableColumns(120, "codex", "claude")))
		if strings.Contains(row, "↳") || strings.Contains(row, "[subagent]") {
			t.Fatalf("row duplicates its Session tree: %s", row)
		}
	}
}

func TestSessionTreeBoundsCyclesAndDeepAncestry(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{Provider: "codex", ID: "a", ParentID: "b", Name: "A"},
		{Provider: "codex", ID: "b", ParentID: "a", Name: "B"},
	}
	for _, session := range model.sessions {
		if name := model.listSessionName(session); lipgloss.Width(name) > maxSessionTreeDepth*3+1 {
			t.Fatalf("cycle exceeded the bounded tree gutter: %q", name)
		}
	}

	model.sessions = nil
	for depth := range maxSessionTreeDepth + 3 {
		session := agent.Session{Provider: "codex", ID: fmt.Sprint(depth), Name: "Task"}
		if depth > 0 {
			session.ParentID = fmt.Sprint(depth - 1)
		}
		model.sessions = append(model.sessions, session)
	}
	last := model.sessions[len(model.sessions)-1]
	if name := model.listSessionName(last); lipgloss.Width(name) != maxSessionTreeDepth*3+len(last.Name) {
		t.Fatalf("deep ancestry did not preserve a bounded task name: %q", name)
	}
}

func TestSessionTreeUnicodeTitlesKeepMarkerAndColumnAlignment(t *testing.T) {
	model := NewModel(nil)
	parent := agent.Session{Provider: "codex", ID: "parent", Name: "Main task"}
	child := agent.Session{Provider: "codex", ID: "child", ParentID: parent.ID, Name: "界面 navigation"}
	model.sessions = []agent.Session{parent, child}
	identity := sessionIdentity{provider: child.Provider, id: child.ID}
	for _, width := range []int{30, 62, 100, 150} {
		columns := tableColumns(width, "codex")
		model.unreadSessions = map[sessionIdentity]bool{identity: true}
		unread := ansi.Strip(model.sessionRowView(child, true, columns))
		model.unreadSessions = nil
		read := ansi.Strip(model.sessionRowView(child, false, columns))
		readGutter := ansi.Cut(read, 0, selectionCursorWidth)
		if lipgloss.Width(unread) != lipgloss.Width(read) || strings.Contains(readGutter, indicatorDot) {
			t.Fatalf("width %d: clearing a marker moved or resized the row:\n%q\n%q", width, unread, read)
		}
		// Compare terminal columns rather than bytes because the marker is Unicode.
		if ansi.Cut(unread, selectionCursorWidth, lipgloss.Width(unread)) !=
			ansi.Cut(read, selectionCursorWidth, lipgloss.Width(read)) {
			t.Fatalf("width %d: marker changes shifted task columns", width)
		}
	}
}
