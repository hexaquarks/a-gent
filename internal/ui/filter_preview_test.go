package ui

import (
	"testing"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSidebarPreviewsRestoreConfirmedFiltersOnExit(t *testing.T) {
	for _, kind := range []string{"view", "provider", "project"} {
		t.Run(kind, func(t *testing.T) {
			model := NewModel(nil)
			model.sessions = []agent.Session{
				{ID: "one", Provider: "claude", State: agent.StateRunning, WorkingDirectory: "/one"},
				{ID: "two", Provider: "codex", State: agent.StateIdle, WorkingDirectory: "/two"},
			}
			model.updateTableRows()
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
			target := 1
			if kind == "provider" {
				target = len(sidebarViews())
			}
			if kind == "project" {
				target = model.projectItemStart()
			}
			model.sidebarCursor = target
			model.moveSidebarCursor(0)
			if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id != "one" {
				t.Fatal("focused filter was not previewed")
			}
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
			if len(model.table.Rows()) != 2 {
				t.Fatal("unconfirmed preview persisted after leaving sidebar")
			}

			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
			if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id != "one" {
				t.Fatal("confirmed filter was lost on exit")
			}

			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
			if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id != "two" {
				t.Fatal("next filter did not preview")
			}
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
			if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id != "one" {
				t.Fatal("preview overwrote the confirmed selection")
			}
		})
	}
}
