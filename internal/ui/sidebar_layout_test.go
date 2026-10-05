package ui

import (
	"strings"
	"testing"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestSidebarLabelsAndCountsAlign(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{Provider: "codex", WorkingDirectory: "/projects/a-gent", State: agent.StateRunning}}
	model.pinnedProjects = map[string]bool{"/projects/a-gent": true}
	for _, focus := range []bool{false, true} {
		model.sidebarFocus = focus
		for index, item := range model.sidebarItems() {
			model.sidebarCursor = index
			row := ansi.Strip(model.sidebarItemView(item, index, 1))
			if !strings.HasPrefix(row, item.label) {
				t.Fatalf("label is indented: %q", row)
			}
			if !strings.HasSuffix(row, "  1") || lipgloss.Width(row) != model.sidebarWidth() {
				t.Fatalf("count is misaligned: %q", row)
			}
		}
	}
}

func TestProjectsStayAnchoredWhileFilteringAndRefreshing(t *testing.T) {
	for _, height := range []int{PopupContentHeight, 32} {
		model := projectTestModel()
		updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: height})
		model = updated.(Model)
		projectLine := -1
		check := func() {
			t.Helper()
			lines := strings.Split(ansi.Strip(model.renderSidebar(summarizeSessions(model.sessions))), "\n")
			for index, line := range lines {
				if strings.HasPrefix(line, "PROJECTS") {
					if projectLine == -1 {
						projectLine = index
					}
					if index != projectLine {
						t.Fatalf("project heading moved from %d to %d", projectLine, index)
					}
				}
			}
			if len(lines) != model.sidebarHeight() {
				t.Fatalf("sidebar height changed: %d, want %d", len(lines), model.sidebarHeight())
			}
			if !strings.HasPrefix(lines[len(lines)-1], "/") {
				t.Fatal("search hint is not at the bottom")
			}
			if got := lipgloss.Height(model.View()); got != height {
				t.Fatalf("dashboard height = %d, want %d", got, height)
			}
		}
		check()
		model.projectSearching = true
		model.projectQuery = "project-79"
		check()
		model.projectQuery = "missing"
		check()
		model.projectSearching = false
		model.sessions = model.sessions[:1]
		model.updateTableRows()
		check()
		model.sessions = nil
		model.updateTableRows()
		check()
	}
}

func TestAgentsHaveTopSpacingAndKeepFocusedTypeVisible(t *testing.T) {
	model := NewModel(nil)
	for _, provider := range []string{"claude", "codex", "gemini", "other"} {
		model.sessions = append(model.sessions, agent.Session{Provider: provider})
	}
	model.agentsExpanded = true
	model.sidebarFocus = true
	model.sidebarCursor = model.projectItemStart() - 1
	sidebar := ansi.Strip(model.renderSidebar(summarizeSessions(model.sessions)))
	if !strings.Contains(sidebar, "\n\nAGENTS") {
		t.Fatal("agents section lacks top padding")
	}
	if !strings.Contains(sidebar, "other") {
		t.Fatal("focused agent is outside the viewport")
	}
}
