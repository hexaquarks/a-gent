package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// Exercise the complete interface with parent/child work, attention requests,
// observed completions, and simultaneous activity and edits.
func interfaceReviewDashboard() Model {
	now := time.Now()
	model := NewModel(nil, WithSessionNavigator(&fakeNavigator{}))
	model.sessions = []agent.Session{
		{
			ID: "focus", Provider: "codex", Name: "Fix sidebar focus",
			WorkingDirectory: "/projects/asteria", State: agent.StateRunning,
		},
		{
			ID: "navigation", ParentID: "focus", Provider: "codex", Name: "Review navigation",
			WorkingDirectory: "/projects/asteria", State: agent.StateRunning,
		},
		{
			ID: "keyboard", ParentID: "focus", Provider: "codex", Name: "Test keyboard traversal",
			WorkingDirectory: "/projects/asteria", State: agent.StateIdle,
		},
		{
			ID: "cards", Provider: "claude", Name: "Polish project cards",
			WorkingDirectory: "/projects/paperboat", State: agent.StateRunning,
		},
		{
			ID: "deploy", Provider: "opencode", Name: "Add deployment checks",
			WorkingDirectory: "/projects/copperline", State: agent.StateRunning,
		},
		{
			ID: "config", ParentID: "deploy", Provider: "opencode", Name: "Check deployment config",
			WorkingDirectory: "/projects/copperline", State: agent.StateRunning,
		},
		{
			ID: "ci", ParentID: "deploy", Provider: "opencode", Name: "Review CI failures",
			WorkingDirectory: "/projects/copperline", State: agent.StateIdle,
		},
		{
			ID: "retry", Provider: "claude", Name: "Review retry handling",
			WorkingDirectory: "/projects/asteria", State: agent.StateWaiting,
		},
		{
			ID: "timer", Provider: "pi", Name: "Track time per move",
			WorkingDirectory: "/projects/sundial", State: agent.StateIdle,
		},
		{
			ID: "api", Provider: "claude", Name: "Check API examples",
			WorkingDirectory: "/projects/copperline", State: agent.StateRunning,
		},
		{
			ID: "map", Provider: "codex", Name: "Show trip stops on map",
			WorkingDirectory: "/projects/moonbeam", State: agent.StateIdle,
		},
		{
			ID: "search", Provider: "claude", Name: "Improve search tests",
			WorkingDirectory: "/projects/moonbeam", State: agent.StateIdle,
		},
		{
			ID: "errors", Provider: "claude", Name: "Review error messages",
			WorkingDirectory: "/projects/paperboat", State: agent.StateWaiting,
		},
		{
			ID: "results", Provider: "pi", Name: "Summarize test results",
			WorkingDirectory: "/projects/sundial", State: agent.StateIdle,
		},
	}
	for index := range model.sessions {
		model.sessions[index].LastActiveAt = now.Add(-time.Duration(index+1) * time.Minute)
	}
	model.sessions[0].LastActiveAt = now.Add(-19 * time.Second)
	model.sessions[1].LastActiveAt = now.Add(-30 * time.Second)
	model.unreadSessions = map[sessionIdentity]bool{
		{provider: "claude", id: "retry"}:  true,
		{provider: "codex", id: "map"}:     true,
		{provider: "claude", id: "search"}: true,
	}
	model.finishedSessions = map[sessionIdentity]time.Time{
		{provider: "codex", id: "map"}:     now,
		{provider: "claude", id: "search"}: now,
	}
	model.previewKey = sessionIdentity{provider: "codex", id: "focus"}
	model.previewSources = map[string]agent.PreviewSource{"codex": &previewSource{}}
	model.previewCache = map[sessionIdentity]previewEntry{model.previewKey: {
		loaded: true, checked: now,
		activity: &agent.Activity{
			Label: "Testing", Text: "Keyboard navigation passes.\nReviewing focus changes.", At: now,
		},
		edit: &agent.Edit{
			Filename: "internal/ui/sidebar.go", CompletedAt: now,
			Diff: "@@ -42,3 +42,6 @@\n-    return model.focus\n+    return model.focusedPanel\n }\n \n" +
				"+func (model Model) sidebarHasFocus() bool {\n+    return model.focusedPanel == focusSidebar\n+}",
		},
	}}
	return model
}

func TestDashboardInterfaceLayouts(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)
	artifacts := os.Getenv("A_GENT_TEST_ARTIFACTS")

	for _, size := range [][2]int{{160, 40}, {120, 29}, {80, 24}, {40, 16}} {
		for _, board := range []bool{false, true} {
			layout := "list"
			if board {
				layout = "board"
			}
			t.Run(fmt.Sprintf("%s-%dx%d", layout, size[0], size[1]), func(t *testing.T) {
				model := interfaceReviewDashboard()
				model.boardVisible = board
				updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				model = updated.(Model)
				view := model.View()
				plain := ansi.Strip(view)
				if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
					t.Fatalf("dashboard exceeds its terminal: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
				}
				if !strings.Contains(plain, "Fix sidebar focus") {
					t.Fatal("mixed work hid the selected task")
				}
				if size[0] == 160 {
					for _, text := range []string{"10 sessions · 4 subagents", "Needs you", "Unseen", "sidebar.go"} {
						if !strings.Contains(plain, text) {
							t.Errorf("complete dashboard omitted %q", text)
						}
					}
					if board {
						if strings.Contains(plain, "Review navigation") || !strings.Contains(plain, "Finished this visit") {
							t.Fatal("Board lost its top-level work and completion summary")
						}
					} else if !strings.Contains(plain, "├─ Review navigation") ||
						!strings.Contains(plain, "└─ Test keyboard traversal") ||
						!strings.Contains(plain, "├─ codex") || !strings.Contains(plain, "└─ codex") {
						t.Fatal("List lost its aligned subagent trees")
					}
				}
				if artifacts != "" {
					t.Setenv("A_GENT_TEST_ARTIFACTS", filepath.Join(artifacts, layout, fmt.Sprintf("%dx%d", size[0], size[1])))
					writeReferenceCapture(t, view, plain)
				}
			})
		}
	}
}
