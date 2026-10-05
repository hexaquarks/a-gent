package ui

import (
	"fmt"
	"strings"
	"testing"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestProviderColorsAcrossDashboard(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)
	for _, provider := range []string{"codex", "claude", "other"} {
		model := NewModel(nil)
		model.sessions = []agent.Session{{Provider: provider, Name: "Example"}}
		_, style := sessionColumnValue(model.sessions[0], "Agent")
		if style.GetForeground() != providerStyle(provider).GetForeground() {
			t.Fatal("table uses a different provider color")
		}
		colorSample := providerStyle(provider).Render(provider)
		// The same escape prefix must appear in sidebar and selected-session text.
		prefix := strings.TrimSuffix(strings.Split(colorSample, provider)[0], "m")
		for _, view := range []string{model.renderSidebar(summarizeSessions(model.sessions)), model.detailView(), model.sessionTableView()} {
			if !strings.Contains(view, prefix) {
				t.Fatalf("%s color missing from %q", provider, view)
			}
		}
	}
	if providerStyle("codex").GetForeground() == providerStyle("claude").GetForeground() {
		t.Fatal("known providers share a color")
	}
}

func TestAgentSectionCollapseExpandAndFilter(t *testing.T) {
	model := NewModel(nil)
	for _, provider := range []string{"claude", "codex", "gemini", "other"} {
		model.sessions = append(model.sessions, agent.Session{ID: provider, Provider: provider})
	}
	sidebar := model.renderSidebar(summarizeSessions(model.sessions))
	if !strings.Contains(sidebar, "AGENTS (4)") || !strings.Contains(sidebar, "All types") || strings.Contains(sidebar, "gemini") {
		t.Fatalf("unexpected collapsed agents: %s", sidebar)
	}
	model.sidebarFocus = true
	model.sidebarCursor = len(sidebarViews())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !strings.Contains(model.renderSidebar(summarizeSessions(model.sessions)), "claude") {
		t.Fatal("enter did not expand providers")
	}
	model.moveSidebarCursor(1)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.selectedProvider != "claude" || len(model.filteredSessions()) != 1 {
		t.Fatal("provider selection did not filter sessions")
	}
	model.moveSidebarCursor(-1)
	if model.selectedProvider != "claude" {
		t.Fatal("browsing All types cleared the filter without Enter")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if len(model.filteredSessions()) != 4 {
		t.Fatal("All types did not clear provider filter")
	}
	model.sessions = model.sessions[:2]
	if strings.Contains(model.renderSidebar(summarizeSessions(model.sessions)), "All types") {
		t.Fatal("small provider list remains collapsed")
	}
}

func TestScrollbarTracksVisibleSessionsWithoutChangingWidth(t *testing.T) {
	model := NewModel(nil)
	for index := 0; index < 30; index++ {
		model.sessions = append(model.sessions, agent.Session{ID: fmt.Sprint(index), Name: "Example"})
	}
	model.updateTableRows()
	first := strings.Split(ansi.Strip(model.sessionTableView()), "\n")
	if !strings.HasSuffix(first[2], "┃") {
		t.Fatal("scroll thumb does not start at top")
	}
	model.table.SetCursor(29)
	last := strings.Split(ansi.Strip(model.sessionTableView()), "\n")
	if !strings.HasSuffix(last[len(last)-1], "┃") || !strings.HasSuffix(last[2], "│") {
		t.Fatal("scroll thumb did not reach bottom")
	}
	for _, row := range last {
		if row != "" && lipgloss.Width(row) != lipgloss.Width(first[0]) {
			t.Fatal("scrollbar changes row width")
		}
	}
	model.sessions = model.sessions[:1]
	model.updateTableRows()
	if strings.Contains(model.sessionTableView(), "┃") {
		t.Fatal("short list renders scrollbar")
	}
}

func TestBrowsingAgentsRequiresEnterToSelectAndClearFilter(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "claude-session", Provider: "claude", State: agent.StateIdle},
		{ID: "codex-session", Provider: "codex", State: agent.StateIdle},
	}
	model.updateTableRows()
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyTab})
	model.sidebarCursor = len(sidebarViews()) - 1
	for range 2 {
		model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
		if model.selectedProvider != "" || len(model.table.Rows()) != 1 {
			t.Fatal("browsing an agent did not preview without committing")
		}
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.selectedProvider != "codex" || len(model.table.Rows()) != 1 {
		t.Fatal("Enter did not select the focused agent")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyUp})
	if model.selectedProvider != "codex" {
		t.Fatal("browsing changed the selected agent")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.selectedProvider != "claude" || len(model.table.Rows()) != 1 {
		t.Fatal("Enter did not switch to the focused agent")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.selectedProvider != "" || len(model.table.Rows()) != 2 {
		t.Fatal("Enter on the selected agent did not clear the filter")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	if model.selectedProvider != "" {
		t.Fatal("browsing past the agents left a provider filter")
	}
}
