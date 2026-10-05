package ui

import (
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"
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
			for range target {
				model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
			}
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

func TestPreviewPreservesOtherConfirmedFilters(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "claude-running", Provider: "claude", State: agent.StateRunning, WorkingDirectory: "/one"},
		{ID: "claude-idle", Provider: "claude", State: agent.StateIdle, WorkingDirectory: "/two"},
		{ID: "codex-running", Provider: "codex", State: agent.StateRunning, WorkingDirectory: "/two"},
		{ID: "codex-idle", Provider: "codex", State: agent.StateIdle, WorkingDirectory: "/one"},
	}
	model.updateTableRows()
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	// Confirm Active, then browse through the other views to Codex.
	for range 4 {
		model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	}
	if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id != "codex-running" {
		t.Fatal("agent preview did not retain the confirmed Active view")
	}
	if model.selectedView != activeView || model.selectedProvider != "" {
		t.Fatal("preview changed confirmed filters")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	if len(model.table.Rows()) != 2 {
		t.Fatal("leaving preview did not restore Active across all agents")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	if model.selectedView != activeView || model.selectedProvider != "codex" || len(model.table.Rows()) != 1 {
		t.Fatal("Enter did not retain both confirmed filters")
	}
	// Preview and clear the confirmed agent, then leave the sidebar.
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	if model.selectedView != activeView || model.selectedProvider != "" || len(model.table.Rows()) != 2 {
		t.Fatal("clearing an agent also cleared the confirmed view")
	}
}

func TestPreviewUpdatesWithLiveSessionsWithoutBeingConfirmed(t *testing.T) {
	model := NewModel(nil)
	updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{
		{ID: "idle", State: agent.StateIdle},
	}})
	model = updated.(Model)
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	if len(model.table.Rows()) != 0 {
		t.Fatal("empty Active preview still shows idle sessions")
	}
	if _, ok := model.selectedSession(); ok {
		t.Fatal("empty preview retains a selected session")
	}
	updated, _ = model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{
		{ID: "idle", State: agent.StateIdle},
		{ID: "running", State: agent.StateRunning},
	}})
	model = updated.(Model)
	if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id != "running" {
		t.Fatal("live update did not refresh the preview")
	}
	if model.selectedView != allView {
		t.Fatal("live update committed the preview")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	if len(model.table.Rows()) != 2 {
		t.Fatal("leaving preview did not restore refreshed All sessions")
	}
}

func TestProjectSearchDiscardsPreviewAndKeepsConfirmedFiltersOnCancel(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "running", Provider: "codex", State: agent.StateRunning, WorkingDirectory: "/one"},
		{ID: "idle", Provider: "codex", State: agent.StateIdle, WorkingDirectory: "/two"},
	}
	model.updateTableRows()
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	if model.tableSessionIDs[0].id != "idle" {
		t.Fatal("Recent preview did not replace Active in the grid")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if model.tableSessionIDs[0].id != "running" {
		t.Fatal("opening search retained an unconfirmed preview")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("two")})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	if model.selectedView != activeView || model.selectedProject != "" || model.tableSessionIDs[0].id != "running" {
		t.Fatal("canceling search changed the confirmed filter")
	}
}
