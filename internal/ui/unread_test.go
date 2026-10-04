package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func refreshUnreadTestModel(model Model, sessions ...agent.Session) Model {
	updated, _ := model.Update(sessionsUpdatedMessage{sessions: sessions})
	return updated.(Model)
}

func TestUnreadSessionStateTransitions(t *testing.T) {
	for _, state := range []agent.State{agent.StateWaiting, agent.StateIdle, agent.StateError, agent.StateUnavailable, ""} {
		t.Run(string(state), func(t *testing.T) {
			session := agent.Session{ID: "one", Provider: "codex", State: state}
			identity := sessionIdentity{provider: session.Provider, id: session.ID}
			model := refreshUnreadTestModel(NewModel(nil), session)
			if len(model.unreadSessions) != 0 {
				t.Fatal("initial non-running session was marked unread")
			}
			session.State = agent.StateRunning
			model = refreshUnreadTestModel(model, session)
			session.State = state
			model = refreshUnreadTestModel(model, session)
			if !model.unreadSessions[identity] {
				t.Fatal("leaving Running did not mark the selected session unread")
			}
			model = refreshUnreadTestModel(model, session)
			if !model.unreadSessions[identity] {
				t.Fatal("refresh cleared an unread session")
			}
			failed, _ := model.Update(sessionsUpdatedMessage{err: errors.New("offline")})
			model = failed.(Model)
			if !model.unreadSessions[identity] {
				t.Fatal("failed refresh cleared an unread session")
			}
			model.markSelectedSessionRead()
			model = refreshUnreadTestModel(model, session)
			if model.unreadSessions[identity] {
				t.Fatal("unchanged state restored a dismissed dot")
			}
			session.State = agent.StateRunning
			model = refreshUnreadTestModel(model, session)
			session.State = state
			model = refreshUnreadTestModel(model, session)
			if !model.unreadSessions[identity] {
				t.Fatal("second run did not restore the unread indicator")
			}
			session.State = agent.StateRunning
			model = refreshUnreadTestModel(model, session)
			if model.unreadSessions[identity] {
				t.Fatal("running session retained the completed-run indicator")
			}
		})
	}
}

func TestUnreadSessionsKeepProviderIdentityAndRemoveMissingSessions(t *testing.T) {
	codex := agent.Session{ID: "same", Provider: "codex", State: agent.StateRunning}
	claude := agent.Session{ID: "same", Provider: "claude", State: agent.StateIdle}
	model := refreshUnreadTestModel(NewModel(nil), codex, claude)
	codex.State = agent.StateWaiting
	model = refreshUnreadTestModel(model, claude, codex)
	if len(model.unreadSessions) != 1 || !model.unreadSessions[sessionIdentity{provider: "codex", id: "same"}] {
		t.Fatalf("unread sessions confused providers: %v", model.unreadSessions)
	}
	model = refreshUnreadTestModel(model, claude)
	model = refreshUnreadTestModel(model, codex, claude)
	if len(model.unreadSessions) != 0 {
		t.Fatal("removed session retained its old unread indicator")
	}
}

func TestUnreadDotSurvivesSortingAndFilteringUntilKeyboardSelection(t *testing.T) {
	first := agent.Session{ID: "a", Provider: "codex", Name: "Alpha", State: agent.StateRunning}
	second := agent.Session{ID: "b", Provider: "codex", Name: "Beta", State: agent.StateRunning}
	model := refreshUnreadTestModel(NewModel(nil), first, second)
	second.State = agent.StateIdle
	model = refreshUnreadTestModel(model, first, second)
	model.selectedView = activeView
	model.updateTableRows()
	model.selectedView = allView
	model.sort = sessionSort{column: "Session", descending: true}
	model.updateTableRows()
	identity := sessionIdentity{provider: "codex", id: "b"}
	if !model.unreadSessions[identity] || !strings.Contains(ansi.Strip(model.sessionRowView(0, model.table.Columns())), "● Beta") {
		t.Fatal("sorting or filtering lost the unread dot")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.unreadSessions[identity] {
		t.Fatal("keyboard selection did not clear the dot")
	}
}

func TestHoverPreservesUnreadSessionDots(t *testing.T) {
	for _, width := range []int{62, 100, 150} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			model := NewModel(nil)
			updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: PopupContentHeight})
			model = updated.(Model)
			sessions := make([]agent.Session, maximumSessionRows+2)
			for index := range sessions {
				sessions[index] = agent.Session{
					ID: fmt.Sprintf("%02d", index), Provider: "codex",
					Name: fmt.Sprintf("Task%02d", index), State: agent.StateRunning,
				}
			}
			model = refreshUnreadTestModel(model, sessions...)
			sessions = slices.Clone(sessions)
			for index := range sessions {
				sessions[index].State = agent.StateIdle
			}
			model = refreshUnreadTestModel(model, sessions...)
			model.table.SetCursor(maximumSessionRows)

			// Hover a visible session row in the rendered screen output.
			x, y := -1, -1
			for lineNumber, line := range strings.Split(ansi.Strip(model.View()), "\n") {
				if offset := strings.Index(line, "codex"); offset >= 0 {
					x = lipgloss.Width(line[:offset])
					y = lineNumber
					break
				}
			}
			if x < 0 {
				t.Fatal("no session row rendered")
			}
			for _, point := range [][2]int{{0, y}, {x, y - 1}, {lipgloss.Width(model.View()), y}, {x, y + model.table.Height()}} {
				updated, _ = model.Update(tea.MouseMsg{X: point[0], Y: point[1], Action: tea.MouseActionMotion})
				model = updated.(Model)
				if len(model.unreadSessions) != len(sessions) {
					t.Fatalf("point %v cleared a dot; row at %d,%d, height %d:\n%s", point, x, y, model.table.Height(), ansi.Strip(model.View()))
				}
			}
			if len(model.unreadSessions) != len(sessions) {
				t.Fatal("hover outside session rows cleared a dot")
			}
			updated, _ = model.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion})
			model = updated.(Model)
			if len(model.unreadSessions) != len(sessions) {
				t.Fatalf("hover cleared an unread dot: %v", model.unreadSessions)
			}
		})
	}
}

func TestUnreadDotKeepsSessionNamesAligned(t *testing.T) {
	model := NewModel(nil)
	session := agent.Session{ID: "one", Provider: "codex", Name: "Example"}
	model.unreadSessions = map[sessionIdentity]bool{{provider: "codex", id: "one"}: true}
	unread := ansi.Strip(model.sessionNameCell(session, 16, lipgloss.Color(colorSelection)))
	model.unreadSessions = nil
	read := ansi.Strip(model.sessionNameCell(session, 16, lipgloss.Color(colorSelection)))
	if lipgloss.Width(strings.Split(unread, "Example")[0]) != lipgloss.Width(strings.Split(read, "Example")[0]) || lipgloss.Width(unread) != 16 || lipgloss.Width(read) != 16 {
		t.Fatalf("clearing dot shifted or resized the name: %q, %q", unread, read)
	}
}
