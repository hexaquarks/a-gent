package ui

import (
	"fmt"
	"testing"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestNarrowSidebarBoundaryKeepsSubagentColumnsReadable(t *testing.T) {
	for _, provider := range []string{"opencode", "long-provider-name"} {
		t.Run(provider, func(t *testing.T) {
			model := NewModel(nil)
			model.sessions = []agent.Session{
				{ID: "parent", Provider: provider, Name: "Parent task", State: agent.StateRunning},
				{ID: "child", Provider: provider, ParentID: "parent", Name: "Child task", State: agent.StateWaiting},
			}
			model.resizeTable()
			model.table.SetCursor(1)
			for _, width := range []int{64, 65, 66, 67, 68, 69, 68, 67, 66, 65, 64} {
				t.Run(fmt.Sprint(width), func(t *testing.T) {
					updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: PopupHeight})
					model = updated.(Model)
					columns := model.table.Columns()
					for _, column := range columns {
						if column.Width <= 0 {
							t.Fatalf("width %d has unusable %s column: %d", width, column.Title, column.Width)
						}
					}

					// OpenCode fits a minimal status at 68; longer providers need
					// the task-only fallback throughout this sidebar boundary.
					needsTaskOnly := width >= 65 && (provider == "long-provider-name" || width < 68)
					if needsTaskOnly && (len(columns) != 1 || columns[0].Title != "Session") {
						t.Fatalf("width %d squeezed status instead of keeping the task: %+v", width, columns)
					}
					if !needsTaskOnly && width >= 65 && (len(columns) != 3 || columns[1].Title != "Status") {
						t.Fatalf("width %d did not restore status when space returned: %+v", width, columns)
					}
					selected, ok := model.selectedSession()
					if !ok || selected.ID != "child" {
						t.Fatalf("resize lost selected child: %+v", selected)
					}
					view := model.View()
					if lipgloss.Width(view) > width || lipgloss.Height(view) > PopupHeight {
						t.Fatalf("width %d dashboard exceeds its terminal", width)
					}
				})
			}
		})
	}
}

func TestSessionColumnWidthsFitLongProvidersAcrossBoundaries(t *testing.T) {
	model := NewModel(nil)
	child := agent.Session{
		ID: "child", ParentID: "parent", Provider: "long-provider-name",
		Name: "Child task", State: agent.StateWaiting,
	}
	model.sessions = []agent.Session{
		{ID: "parent", Provider: child.Provider, Name: "Parent task", State: agent.StateRunning},
		child,
	}
	for width := 24; width <= 90; width++ {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			// Match resizeTable's allowance for a branch before provider names.
			columns := tableColumns(width, "   "+child.Provider)
			for _, column := range columns {
				if column.Width <= 0 {
					t.Fatalf("table width %d has unusable %s width %d", width, column.Title, column.Width)
				}
			}
			row := model.sessionRowView(child, false, columns)
			availableWidth := width - panelStyle.GetHorizontalFrameSize() - scrollbarGutterWidth
			if lipgloss.Width(row) != availableWidth {
				t.Fatalf("table width %d rendered %d cells, want %d", width, lipgloss.Width(row), availableWidth)
			}
		})
	}
}
