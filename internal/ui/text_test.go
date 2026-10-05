package ui

import (
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestProviderTextCannotSendTerminalCommands(t *testing.T) {
	controls := "\x1b[2J\x1b]52;c;cGF5bG9hZA==\a\x1b]0;injected-title\a"
	session := agent.Session{
		ID:               "session" + controls,
		Name:             "Review" + controls + "\nnext",
		WorkingDirectory: "/projects/project" + controls,
		State:            agent.State("idle" + controls),
	}
	provider := "claude" + controls
	for _, width := range []int{62, 100, 150} {
		model := NewModel(nil)
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: PopupContentHeight})
		model = updated.(Model)
		model.applyProviderUpdate(polling.Update{Provider: provider, Sessions: []agent.Session{session}})

		for name, view := range map[string]string{
			"dashboard": model.View(),
			"table":     model.sessionTableView(),
			"details":   model.detailView(),
			"sidebar":   model.renderSidebar(summarizeSessions(model.sessions)),
		} {
			for _, unsafe := range []string{"\x1b[2J", "\x1b]52;", "\x1b]0;", "cGF5bG9hZA==", "injected-title"} {
				if strings.Contains(view, unsafe) {
					t.Errorf("width %d: %s contains provider-supplied terminal commands", width, name)
					break
				}
			}
		}
		if details := ansi.Strip(model.detailView()); !strings.Contains(details, "Review next") {
			t.Error("session name was not kept on one line")
		}
	}
}

func TestDisplayCleanupPreservesSessionDataForNavigation(t *testing.T) {
	navigator := &fakeNavigator{}
	model := NewModel(nil, WithSessionNavigator(navigator))
	session := agent.Session{
		ID:               "session\noriginal",
		Name:             "Révision 🐈\x1b[31m rouge\x1b[0m",
		WorkingDirectory: "/projects/project\noriginal",
		State:            agent.StateIdle,
	}
	model.applyProviderUpdate(polling.Update{Provider: "claude", Sessions: []agent.Session{session}})
	if !strings.Contains(ansi.Strip(model.detailView()), "Révision 🐈 rouge") {
		t.Fatal("display cleanup lost readable Unicode text")
	}
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	command()
	if navigator.session.ID != session.ID || navigator.session.WorkingDirectory != session.WorkingDirectory {
		t.Fatal("display cleanup changed the session used for navigation")
	}
}
