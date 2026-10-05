package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func refreshUnreadTestModel(model Model, sessions ...agent.Session) Model {
	// Existing rendering tests supply whole snapshots. Deliver each provider
	// separately, including successful empty results for removed providers.
	providers := make(map[string][]agent.Session)
	for _, session := range model.sessions {
		providers[session.Provider] = nil
	}
	for _, session := range sessions {
		providers[session.Provider] = append(providers[session.Provider], session)
	}
	for provider, sessions := range providers {
		updated, _ := model.Update(polling.Update{Provider: provider, Sessions: sessions})
		model = updated.(Model)
	}
	return model
}

func TestUnseenLabelIsConditionalAndRightAligned(t *testing.T) {
	for _, width := range []int{62, 100, 150} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			model := NewModel(nil)
			updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: PopupContentHeight})
			model = updated.(Model)
			first := agent.Session{ID: "a", Provider: "codex", Name: "Alpha", State: agent.StateRunning}
			second := agent.Session{ID: "b", Provider: "codex", Name: "Beta", State: agent.StateIdle}
			model = refreshUnreadTestModel(model, first, second)
			if strings.Contains(model.detailView(), "Unseen state change") {
				t.Fatal("read session shows the unseen label")
			}
			first.State = agent.StateIdle
			model = refreshUnreadTestModel(model, first, second)
			panel := ansi.Strip(detailStyle.Width(model.table.Width()).Render(model.detailView()))
			lines := strings.Split(panel, "\n")
			labelRow := detailStyle.GetBorderTopSize() + detailStyle.GetPaddingTop()
			if !strings.HasSuffix(lines[labelRow], "● Unseen state change"+strings.Repeat(" ", detailStyle.GetPaddingRight())) {
				t.Fatalf("unseen label is not at the top right:\n%s", panel)
			}
			if lipgloss.Width(lines[labelRow]) != model.table.Width() {
				t.Fatalf("heading overflowed its panel: %q", lines[labelRow])
			}
			model.table.SetCursor(1)
			if strings.Contains(model.detailView(), "Unseen state change") {
				t.Fatal("another session's unread dot shows a label on a read session")
			}
			model.table.SetCursor(0)
			model.markSelectedSessionRead()
			if strings.Contains(model.detailView(), "Unseen state change") {
				t.Fatal("acknowledging the session left the unseen label visible")
			}
			model = refreshUnreadTestModel(model)
			if strings.Contains(model.detailView(), "Unseen state change") {
				t.Fatal("empty panel shows the unseen label")
			}
		})
	}
}

type unreadInputSnapshot struct {
	message tea.Msg
	view    string
	unread  int
}

type unreadInputModel struct {
	Model
	snapshots chan unreadInputSnapshot
}

func (model unreadInputModel) Init() tea.Cmd { return nil }

func (model unreadInputModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, command := model.Model.Update(message)
	model.Model = updated.(Model)
	switch message.(type) {
	case polling.Update, tea.MouseMsg, tea.KeyMsg:
		model.snapshots <- unreadInputSnapshot{message: message, view: ansi.Strip(model.View()), unread: len(model.unreadSessions)}
	}
	return model, command
}

func TestRawTerminalBrowsingPreservesDotsUntilEnter(t *testing.T) {
	model := NewModel(nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: PopupContentHeight})
	model = updated.(Model)
	first := agent.Session{ID: "a", Provider: "codex", Name: "Alpha", State: agent.StateRunning}
	second := agent.Session{ID: "b", Provider: "codex", Name: "Beta", State: agent.StateIdle}
	model = refreshUnreadTestModel(model, first, second)
	snapshots := make(chan unreadInputSnapshot, 16)
	input, writer := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	program := tea.NewProgram(unreadInputModel{Model: model, snapshots: snapshots},
		tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()
	t.Cleanup(func() {
		program.Quit()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("terminal program failed: %v", err)
			}
		case <-ctx.Done():
			t.Error("terminal program did not stop")
		}
		writer.Close()
		input.Close()
		cancel()
	})
	next := func() unreadInputSnapshot {
		t.Helper()
		select {
		case snapshot := <-snapshots:
			return snapshot
		case <-ctx.Done():
			t.Fatal("terminal input was not processed")
			return unreadInputSnapshot{}
		}
	}
	first.State = agent.StateWaiting
	program.Send(polling.Update{Provider: "codex", Sessions: []agent.Session{first, second}})
	before := next()
	if before.unread != 1 || !strings.Contains(before.view, "● Alpha") || !strings.Contains(before.view, "Unseen state change") {
		t.Fatalf("unread session is not visible before hover:\n%s", before.view)
	}
	for y, line := range strings.Split(before.view, "\n") {
		x := strings.Index(line, "● Alpha")
		if x < 0 {
			continue
		}
		// Feed SGR mouse motion bytes through Bubble Tea's real input decoder.
		column := lipgloss.Width(line[:x]) + 1
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := fmt.Fprintf(writer, "\x1b[<35;%d;%dM", column, y+1); err != nil {
				t.Fatal(err)
			}
			hovered := next()
			if _, ok := hovered.message.(tea.MouseMsg); !ok || hovered.unread != 1 || hovered.view != before.view {
				t.Fatalf("raw hover changed the session dot or panel: %+v", hovered)
			}
		}
		break
	}
	if _, err := io.WriteString(writer, "\x1b[B"); err != nil {
		t.Fatal(err)
	}
	other := next()
	if other.unread != 1 || strings.Contains(other.view, "Unseen state change") {
		t.Fatal("selecting a read session cleared another dot or showed the unseen label")
	}
	if _, err := io.WriteString(writer, "\x1b[A"); err != nil {
		t.Fatal(err)
	}
	read := next()
	if read.unread != 1 || !strings.Contains(read.view, "● Alpha") || !strings.Contains(read.view, "Unseen state change") {
		t.Fatal("moving the keyboard highlight cleared the dot or unseen label")
	}
	if _, err := io.WriteString(writer, "\r"); err != nil {
		t.Fatal(err)
	}
	acknowledged := next()
	if acknowledged.unread != 0 || strings.Contains(acknowledged.view, "● Alpha") || strings.Contains(acknowledged.view, "Unseen state change") {
		t.Fatal("Enter did not clear both dot and label")
	}
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
			failed, _ := model.Update(polling.Update{Provider: "codex", Err: errors.New("offline")})
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

func TestUnreadDotSurvivesSortingFilteringAndKeyboardBrowsing(t *testing.T) {
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
	if !model.unreadSessions[identity] {
		t.Fatal("keyboard browsing cleared the dot")
	}
	for attempt := 0; attempt < 2; attempt++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		model = updated.(Model)
	}
	if !model.unreadSessions[identity] {
		t.Fatal("switching focus cleared the dot")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.unreadSessions[identity] {
		t.Fatal("Enter did not acknowledge the session")
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
