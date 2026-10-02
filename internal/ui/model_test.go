package ui

import (
	"context"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

type fakeAdapter struct {
	sessions []agent.Session
}

func (adapter fakeAdapter) Provider() string {
	return "fake"
}

func (adapter fakeAdapter) Sessions(context.Context) ([]agent.Session, error) {
	return adapter.sessions, nil
}

func TestFetchSessionsUpdatesTheDashboard(t *testing.T) {
	model := NewModel([]agent.Adapter{fakeAdapter{sessions: []agent.Session{{
		ID:               "session-1",
		Provider:         "codex",
		Name:             "Fix dashboard",
		WorkingDirectory: "/projects/a-gent",
		State:            agent.StateRunning,
	}}}})

	message := model.fetchSessions()()
	updatedModel, _ := model.Update(message)
	dashboard := updatedModel.(Model).View()

	if !strings.Contains(dashboard, "Fix dashboard") {
		t.Fatal("dashboard does not render the live session")
	}
	if strings.Contains(dashboard, "mock data") {
		t.Fatal("dashboard still renders mock data")
	}
}

func TestSessionSummary(t *testing.T) {
	summary := summarizeSessions([]agent.Session{
		{State: agent.StateRunning},
		{State: agent.StateWaiting},
		{State: agent.StateIdle},
		{State: agent.StateError},
		{State: agent.StateUnavailable},
	})

	if summary.total != 5 || summary.running != 1 || summary.waiting != 1 || summary.idle != 1 || summary.errors != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestSessionListCapsVisibleRowsAndScrolls(t *testing.T) {
	sessions := make([]agent.Session, maximumSessionRows+2)
	model := NewModel(nil)
	model.sessions = sessions
	model.height = 80
	model.resizeTable()

	if model.table.Height() != maximumSessionRows {
		t.Fatalf("table height = %d, want %d", model.table.Height(), maximumSessionRows)
	}

	model.table.SetCursor(maximumSessionRows)
	start, end := model.visibleSessionRange()
	if start != 1 || end != maximumSessionRows+1 {
		t.Fatalf("visible range = %d-%d, want 1-%d", start, end, maximumSessionRows+1)
	}
}

func TestSessionListRendersEmptyRowPlaceholders(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{Name: "First session"},
		{Name: "Second session"},
		{Name: "Third session"},
	}
	model.height = 80
	model.resizeTable()

	if model.table.Height() != maximumSessionRows {
		t.Fatalf("table height = %d, want %d", model.table.Height(), maximumSessionRows)
	}

	tableView := model.sessionTableView()
	if got, want := strings.Count(tableView, "\n")+1, maximumSessionRows+1; got != want {
		t.Fatalf("rendered table rows = %d, want %d including header", got, want)
	}
}

func TestEmptySessionListUsesAPlaceholderRow(t *testing.T) {
	model := NewModel(nil)
	model.height = 80
	model.resizeTable()

	tableView := model.sessionTableView()
	if !strings.Contains(tableView, "No live sessions found.") {
		t.Fatal("empty session list does not explain that no sessions were found")
	}
	if got, want := strings.Count(tableView, "\n")+1, maximumSessionRows+1; got != want {
		t.Fatalf("rendered table rows = %d, want %d including header", got, want)
	}
}
