package ui

import (
	"slices"
	"testing"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

func TestProjectColorsPreferDistinctAccents(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{Provider: "codex", WorkingDirectory: "/projects/a-gent"},
		{Provider: "codex", WorkingDirectory: "/projects/aiorganize"},
		{Provider: "claude", WorkingDirectory: "/projects"},
		{Provider: "codex", WorkingDirectory: "/projects/a-gent"},
		{Provider: "codex"},
	}
	reserved := []lipgloss.TerminalColor{
		lipgloss.Color(colorAccent), lipgloss.Color(colorRunning),
		lipgloss.Color(colorAttention), lipgloss.Color(colorError),
		providerStyle("codex").GetForeground(), providerStyle("claude").GetForeground(),
	}
	colors := make(map[string]lipgloss.TerminalColor)
	for _, project := range model.projects() {
		if project == "" {
			continue
		}
		color := model.projectStyle(project).GetForeground()
		if slices.Contains(reserved, color) {
			t.Fatalf("project %s reuses an agent, status, or accent color", project)
		}
		for other, otherColor := range colors {
			if color == otherColor {
				t.Fatalf("projects %s and %s share a color", project, other)
			}
		}
		colors[project] = color
	}

	model.selectedProject = "/projects/aiorganize"
	model.pinnedProjects = map[string]bool{"/projects/aiorganize": true}
	slices.Reverse(model.sessions)
	for project, color := range colors {
		if model.projectStyle(project).GetForeground() != color {
			t.Fatalf("filtering, pinning, or reordering changed %s's color", project)
		}
	}
}
