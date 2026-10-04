package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func assertSessionOrder(t *testing.T, model Model, want ...string) {
	t.Helper()
	var got []string
	for _, session := range model.filteredSessions() {
		got = append(got, session.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("session order = %v, want %v", got, want)
	}
}

func sendSortKey(model Model, key tea.KeyMsg) Model {
	updated, _ := model.Update(key)
	return updated.(Model)
}

func runeKey(value rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}}
}

func TestDefaultSortUsesActivityWithUnknownLast(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "unknown"},
		{ID: "old", LastActiveAt: time.Unix(100, 0)},
		{ID: "new", LastActiveAt: time.Unix(300, 0)},
		{ID: "middle", LastActiveAt: time.Unix(200, 0)},
	}
	assertSessionOrder(t, model, "new", "middle", "old", "unknown")
	model.sort.descending = false
	assertSessionOrder(t, model, "old", "middle", "new", "unknown")
	if model.sessions[0].ID != "unknown" {
		t.Fatal("sorting mutated the provider's session slice")
	}
}

func TestSortColumnsAndDirections(t *testing.T) {
	for _, column := range []string{"Session", "Agent", "Status", "Directory"} {
		t.Run(column, func(t *testing.T) {
			model := NewModel(nil)
			model.sessions = []agent.Session{
				{ID: "z", Name: "zebra", Provider: "Zed", State: agent.StateRunning, WorkingDirectory: "/a/zebra"},
				{ID: "a", Name: "Alpha", Provider: "alpha", State: agent.StateError, WorkingDirectory: "/z/Alpha"},
			}
			model.sort = sessionSort{column: column}
			assertSessionOrder(t, model, "a", "z")
			model.sort.descending = true
			assertSessionOrder(t, model, "z", "a")
		})
	}
}

func TestSortTiesAreIndependentOfRefreshOrder(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{ID: "b", Provider: "codex"}, {ID: "a", Provider: "codex"}}
	assertSessionOrder(t, model, "a", "b")
	slices.Reverse(model.sessions)
	assertSessionOrder(t, model, "a", "b")
	model.sort.descending = false
	assertSessionOrder(t, model, "a", "b")
}

func TestRefreshAndSortKeepSelectedSession(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "a", Name: "Alpha", Provider: "codex", LastActiveAt: time.Unix(100, 0)},
		{ID: "b", Name: "Beta", Provider: "codex", LastActiveAt: time.Unix(200, 0)},
	}
	model.updateTableRows()
	model.table.SetCursor(1) // Alpha, currently the older session.
	updated, _ := model.Update(sessionsUpdatedMessage{sessions: []agent.Session{
		{ID: "b", Name: "Beta", Provider: "codex", LastActiveAt: time.Unix(200, 0)},
		{ID: "a", Name: "Alpha", Provider: "codex", LastActiveAt: time.Unix(300, 0)},
	}})
	model = updated.(Model)
	selected, ok := model.selectedSession()
	if !ok || selected.ID != "a" || model.table.Cursor() != 0 {
		t.Fatalf("refresh lost selection: %+v, cursor=%d", selected, model.table.Cursor())
	}
	model.sort = sessionSort{column: "Session", descending: true}
	model.updateTableRows()
	selected, _ = model.selectedSession()
	if selected.ID != "a" || model.table.Cursor() != 1 {
		t.Fatalf("sort lost selection: %+v, cursor=%d", selected, model.table.Cursor())
	}
	updated, _ = model.Update(sessionsUpdatedMessage{sessions: []agent.Session{{ID: "b", Provider: "codex"}}})
	model = updated.(Model)
	if selected, ok = model.selectedSession(); !ok || selected.ID != "b" {
		t.Fatalf("removing selected session left invalid selection: %+v", selected)
	}
	updated, _ = model.Update(sessionsUpdatedMessage{})
	model = updated.(Model)
	if _, ok = model.selectedSession(); ok {
		t.Fatal("empty refresh retained a selected session")
	}
}

func TestSortingHandlesEmptyStartupAndSessionReturn(t *testing.T) {
	model := NewModel(nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: PopupContentHeight})
	model = updated.(Model)
	for _, sessions := range [][]agent.Session{
		nil,
		{{ID: "a", Provider: "codex"}},
		nil,
		{{ID: "b", Provider: "codex"}},
	} {
		updated, _ = model.Update(sessionsUpdatedMessage{sessions: sessions})
		model = updated.(Model)
		_, selected := model.selectedSession()
		if selected != (len(sessions) > 0) {
			t.Fatalf("selection valid = %v with %d sessions", selected, len(sessions))
		}
	}
}

func TestSortKeyCyclesVisibleColumnsAndPreservesSelection(t *testing.T) {
	for _, width := range []int{64, 100, 140} {
		navigator := &fakeNavigator{}
		model := NewModel(nil, WithSessionNavigator(navigator))
		model.sessions = []agent.Session{
			{ID: "a", Name: "Alpha", LastActiveAt: time.Unix(100, 0)},
			{ID: "b", Name: "Beta", LastActiveAt: time.Unix(200, 0)},
		}
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: PopupContentHeight})
		model = updated.(Model)
		model.table.SetCursor(1)
		model.sidebarFocus = true
		initialHeight := lipgloss.Height(model.View())

		for _, column := range model.table.Columns() {
			updated, command := model.Update(runeKey('s'))
			model = updated.(Model)
			if command != nil || navigator.session.ID != "" {
				t.Fatal("cycling sort navigated or closed the dashboard")
			}
			if model.sort.column != column.Title || model.sort.descending != (column.Title == "Last active") {
				t.Fatalf("width %d: sort = %+v, want %s with its default direction", width, model.sort, column.Title)
			}
			if selected, ok := model.selectedSession(); !ok || selected.ID != "a" || !model.sidebarFocus {
				t.Fatalf("width %d: cycling sort lost selection or focus", width)
			}
			if got := lipgloss.Height(model.View()); got != initialHeight {
				t.Fatalf("width %d: cycling sort changed height from %d to %d", width, initialHeight, got)
			}
		}
		if model.sort.column != "Last active" || !model.sort.descending {
			t.Fatal("full cycle did not return to the default sort")
		}
		assertSessionOrder(t, model, "b", "a")
	}
}

func TestCapitalSReversesCurrentSortAndPreservesSelection(t *testing.T) {
	for _, column := range []string{"Last active", "Session", "Agent", "Status", "Directory"} {
		t.Run(column, func(t *testing.T) {
			model := NewModel(nil)
			model.sessions = []agent.Session{
				{ID: "a", Name: "Alpha", Provider: "alpha", State: agent.StateError, WorkingDirectory: "/Alpha", LastActiveAt: time.Unix(100, 0)},
				{ID: "b", Name: "Beta", Provider: "beta", State: agent.StateRunning, WorkingDirectory: "/Beta", LastActiveAt: time.Unix(200, 0)},
			}
			model.sort = sessionSort{column: column}
			model.updateTableRows()
			model.sidebarFocus = true

			for _, descending := range []bool{true, false} {
				updated, command := model.Update(runeKey('S'))
				model = updated.(Model)
				if command != nil || model.sort.column != column || model.sort.descending != descending {
					t.Fatalf("capital S changed the column or failed to reverse: %+v", model.sort)
				}
				if selected, ok := model.selectedSession(); !ok || selected.ID != "a" || !model.sidebarFocus {
					t.Fatal("reversing sort lost selection or focus")
				}
				if descending {
					assertSessionOrder(t, model, "b", "a")
				} else {
					assertSessionOrder(t, model, "a", "b")
				}
				if !strings.Contains(model.sessionHeadingView(), column+" "+model.sort.arrow()) {
					t.Fatal("sort direction indicator did not update")
				}
			}
		})
	}
}

func TestSortIndicatorsFitResponsiveLayouts(t *testing.T) {
	for _, width := range []int{64, 100, 140} {
		model := NewModel(nil)
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: PopupContentHeight})
		model = updated.(Model)
		view := model.View()
		if !strings.Contains(view, "Sort: Last active ↓") || !strings.Contains(view, "Last active ↓") || !strings.Contains(view, "s: sort") || !strings.Contains(view, "S: reverse") {
			t.Fatalf("width %d: missing sort indicators or shortcut:\n%s", width, view)
		}
		for _, column := range model.table.Columns() {
			model.sort = sessionSort{column: column.Title}
			header := ansi.Strip(strings.Split(model.sessionTableView(), "\n")[0])
			if !strings.Contains(header, "↑") {
				t.Fatalf("width %d: header lost arrow for %s: %s", width, column.Title, header)
			}
		}
	}
}

func TestResizeResetsSortWhenColumnIsHidden(t *testing.T) {
	model := NewModel(nil)
	model.sort = sessionSort{column: "Directory"}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 64, Height: PopupContentHeight})
	model = updated.(Model)
	if model.sort.column != "Last active" || !model.sort.descending {
		t.Fatal("resize left the active sort on a hidden column")
	}
}
