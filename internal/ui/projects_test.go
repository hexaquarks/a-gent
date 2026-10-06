package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func projectTestModel() Model {
	model := NewModel(nil)
	for index := 0; index < 80; index++ {
		model.sessions = append(model.sessions, agent.Session{ID: fmt.Sprint(index), Provider: "codex", WorkingDirectory: fmt.Sprintf("/projects/project-%02d", index), State: agent.StateIdle})
	}
	model.updateTableRows()
	return model
}

func sendProjectKey(model Model, key tea.KeyMsg) Model {
	updated, _ := model.Update(key)
	return updated.(Model)
}

func TestProjectListLimitsRowsAndFollowsCursor(t *testing.T) {
	model := projectTestModel()
	sidebar := ansi.Strip(model.renderSidebar(summarizeSessions(model.sessions)))
	if !strings.Contains(sidebar, "PROJECTS (80)") || !strings.Contains(sidebar, fmt.Sprintf("+ %d more…", 80-model.projectRowCapacity())) || strings.Contains(sidebar, fmt.Sprintf("project-%02d", model.projectRowCapacity())) {
		t.Fatalf("unexpected project list: %s", sidebar)
	}
	model.sidebarFocus = true
	model.sidebarCursor = model.projectItemStart() + 79
	sidebar = ansi.Strip(model.renderSidebar(summarizeSessions(model.sessions)))
	if !strings.Contains(sidebar, "project-79") || strings.Contains(sidebar, "project-00") {
		t.Fatal("project viewport did not follow cursor")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("PROJECT-79")})
	if len(model.matchingProjects()) != 1 {
		t.Fatal("search did not find hidden project case-insensitively")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if model.projectSearching || len(model.filteredSessions()) != 1 || model.selectedProject != "/projects/project-79" {
		t.Fatal("search selection did not apply project filter")
	}
}

func TestSearchHandlesEditingCancelAndNoMatches(t *testing.T) {
	model := projectTestModel()
	model.selectedProject = "/projects/project-01"
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q界")})
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyBackspace})
	if model.projectQuery != "q" {
		t.Fatal("backspace did not remove one Unicode character")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if !model.projectSearching || !strings.Contains(model.renderSidebar(summarizeSessions(model.sessions)), "No matching projects") {
		t.Fatal("empty search selected a project")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.selectedProject != "/projects/project-01" || model.projectSearching || len(model.matchingProjects()) != 80 {
		t.Fatal("cancel changed the filter or retained query")
	}
}

func TestProjectPinsPersistAndSortFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "pins.json")
	model := projectTestModel()
	WithProjectPins(path)(&model)
	model.sidebarFocus = true
	model.sidebarCursor = model.projectItemStart() + 79
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if model.projects()[0] != "/projects/project-79" || model.sidebarCursor != model.projectItemStart() {
		t.Fatal("pin did not move project and cursor to top")
	}
	reloaded := NewModel(nil, WithProjectPins(path))
	if !reloaded.pinnedProjects["/projects/project-79"] {
		t.Fatal("pin did not persist")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if model.pinnedProjects["/projects/project-79"] {
		t.Fatal("second press did not unpin")
	}
}

func TestProjectIndicatorsAndNames(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{ID: "run", WorkingDirectory: "/long", State: agent.StateRunning}, {ID: "wait", WorkingDirectory: "/long", State: agent.StateWaiting}}
	_, style := model.projectIndicator("/long")
	if style.GetForeground() != waitingStyle.GetForeground() {
		t.Fatal("running status hides attention")
	}
	model.sessions = model.sessions[:1]
	_, style = model.projectIndicator("/long")
	if style.GetForeground() != runningStyle.GetForeground() {
		t.Fatal("running project lacks green indicator")
	}
	model.sessions[0].State = agent.StateIdle
	model.unreadSessions = map[sessionIdentity]bool{{id: "run"}: true}
	_, style = model.projectIndicator("/long")
	if style.GetForeground() != accentStyle.GetForeground() {
		t.Fatal("unread project lacks cyan indicator")
	}
	row := model.projectItemView(sidebarItem{label: "非常に長いプロジェクト名-example", project: "/long"}, true, true)
	if lipgloss.Width(row) != sidebarContentWidth-sidebarStyle.GetHorizontalPadding() {
		t.Fatalf("project width = %d", lipgloss.Width(row))
	}
}

func TestManyProjectsFitPopup(t *testing.T) {
	model := projectTestModel()
	model.sessions[0].Provider = "claude"
	model.sessions[1].Provider = "gemini"
	model.resizeTable()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: PopupContentHeight})
	if height := lipgloss.Height(updated.(Model).View()); height != PopupContentHeight {
		t.Fatalf("popup height = %d, want %d", height, PopupContentHeight)
	}
}
