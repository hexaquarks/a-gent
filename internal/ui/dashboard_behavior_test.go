package ui

import (
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAcknowledgementKeepsOtherProviderUnread(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "same", Provider: "codex", State: agent.StateIdle},
		{ID: "same", Provider: "claude", State: agent.StateWaiting},
	}
	model.unreadSessions = map[sessionIdentity]bool{
		{provider: "codex", id: "same"}:  true,
		{provider: "claude", id: "same"}: true,
	}
	model.selectedProvider = "codex"
	model.updateTableRows()
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.unreadSessions[sessionIdentity{provider: "codex", id: "same"}] ||
		!model.unreadSessions[sessionIdentity{provider: "claude", id: "same"}] {
		t.Fatal("acknowledgement confused providers with the same session ID")
	}
	if len(model.filteredSessions()) != 1 {
		t.Fatal("acknowledgement removed a session from the provider filter")
	}
}

func TestProviderUpdateReordersSessionsAndPreservesSelection(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "a", Provider: "codex", State: agent.StateRunning, LastActiveAt: time.Unix(200, 0)},
		{ID: "b", Provider: "codex", State: agent.StateIdle, LastActiveAt: time.Unix(100, 0)},
	}
	model.updateTableRows()
	updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{
		{ID: "a", State: agent.StateIdle, LastActiveAt: time.Unix(200, 0)},
		{ID: "b", State: agent.StateRunning, LastActiveAt: time.Unix(300, 0)},
		{ID: "c", State: agent.StateIdle, LastActiveAt: time.Unix(400, 0)},
	}})
	model = updated.(Model)
	assertSessionOrder(t, model, "c", "b", "a")
	if selected, _ := model.selectedSession(); selected.State != agent.StateIdle || len(model.unreadSessions) != 1 {
		t.Fatal("provider update lost the selected session or unread state")
	}
	model.selectedView = activeView
	assertSessionOrder(t, model, "b")
	model.selectedView = allView
	assertSessionOrder(t, model, "c", "b", "a")
	if selected, _ := model.selectedSession(); selected.ID != "a" {
		t.Fatal("provider update lost selection")
	}
}

func TestOpeningAnUpdateUsesTheAcknowledgedSession(t *testing.T) {
	navigator := &fakeNavigator{}
	model := NewModel(nil, WithSessionNavigator(navigator))
	model.sessions = []agent.Session{
		{ID: "a", Provider: "codex", State: agent.StateIdle},
		{ID: "b", Provider: "codex", State: agent.StateIdle},
	}
	model.unreadSessions = map[sessionIdentity]bool{
		{provider: "codex", id: "a"}: true,
		{provider: "codex", id: "b"}: true,
	}
	model.updateTableRows()
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("opening an update did not start navigation")
	}
	defer model.cancelNavigation()

	// A second Enter must not acknowledge another session while navigation is pending.
	updated, duplicate := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if duplicate != nil || len(model.table.Rows()) != 2 || !model.unreadSessions[sessionIdentity{provider: "codex", id: "b"}] {
		t.Fatal("repeated Enter acknowledged another update during navigation")
	}
	command()
	if navigator.session.ID != "a" {
		t.Fatalf("opened %s after acknowledgement; want a", navigator.session.ID)
	}
	if len(model.table.Rows()) != 2 || model.tableSessionIDs[0].id != "a" {
		t.Fatal("acknowledgement changed the selected session")
	}
}
