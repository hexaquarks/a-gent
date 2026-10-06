package ui

import (
	"fmt"
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
			if !strings.HasPrefix(string([]rune(row)[3:]), item.label) {
				t.Fatalf("name does not start after the fixed gutter: %q", row)
			}
			if !strings.HasSuffix(strings.Replace(row, "★", " ", 1), "  1  ") || lipgloss.Width(row) != model.sidebarWidth() {
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
				if strings.HasPrefix(line, " PROJECTS") {
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
	if !strings.Contains(sidebar, "\n\n AGENTS") {
		t.Fatal("agents section lacks top padding")
	}
	if !strings.Contains(sidebar, "other") {
		t.Fatal("focused agent is outside the viewport")
	}
}

func TestSidebarReservesPinAndCountSpaceBeforeTruncatingNames(t *testing.T) {
	for _, count := range []int{1, 42, 999, 1000} {
		for _, name := range []string{"a-very-long-project-name", "長いプロジェクト名-example"} {
			model := NewModel(nil)
			for index := 0; index < count; index++ {
				model.sessions = append(model.sessions, agent.Session{Provider: name, WorkingDirectory: "/" + name, State: agent.StateIdle})
			}
			project := sidebarItem{label: name, project: "/" + name}
			unpinned := ansi.Strip(model.projectItemView(project, false, false))
			model.pinnedProjects = map[string]bool{project.project: true}
			pinned := ansi.Strip(model.projectItemView(project, false, false))
			if strings.Replace(pinned, "★", " ", 1) != unpinned {
				t.Fatalf("pinning moved or truncated another field: %q versus %q", pinned, unpinned)
			}
			countText := fmt.Sprintf("%*d", max(3, len(fmt.Sprint(count))), count)
			for _, row := range []string{
				pinned, unpinned,
				ansi.Strip(model.sidebarItemView(sidebarItem{label: name, provider: name}, 0, count)),
				ansi.Strip(model.sidebarItemView(sidebarItem{label: name, view: activeView}, 0, count)),
			} {
				if lipgloss.Width(row) != model.sidebarWidth() || !strings.HasSuffix(strings.Replace(row, "★", " ", 1), countText+"  ") {
					t.Fatalf("count was truncated or moved: %q", row)
				}
				if !strings.HasPrefix(row, "  ") || !strings.Contains(row, "…") {
					t.Fatalf("missing blank gutter or truncated name: %q", row)
				}
				// Ignore only the pin cell; all sections must allocate the same name width.
				if strings.Replace(row, "★", " ", 1) != unpinned {
					t.Fatalf("sections disagree on reserved columns: %q versus %q", row, unpinned)
				}
			}
			model.sessions[0].State = agent.StateRunning
			active := ansi.Strip(model.projectItemView(project, false, false))
			if strings.TrimPrefix(active, " ● ") != strings.TrimPrefix(pinned, "   ") {
				t.Fatalf("status dot moved another field: %q versus %q", active, pinned)
			}
		}
	}
}
