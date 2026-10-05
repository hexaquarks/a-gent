package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type fakeNavigator struct {
	session agent.Session
	err     error
}

func (navigator *fakeNavigator) Navigate(_ context.Context, session agent.Session) error {
	navigator.session = session
	return navigator.err
}

func TestProviderUpdateRendersTheDashboard(t *testing.T) {
	model := NewModel(nil)
	updatedModel, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{{
		ID: "session-1", Name: "Fix dashboard", WorkingDirectory: "/projects/a-gent", State: agent.StateRunning,
	}}})
	if !strings.Contains(updatedModel.(Model).View(), "Fix dashboard") {
		t.Fatal("dashboard does not render the live session")
	}
}

func TestEnterNavigatesTheSelectedSessionWhenSupported(t *testing.T) {
	navigator := &fakeNavigator{}
	model := NewModel(nil, WithSessionNavigator(navigator))
	model.sessions = []agent.Session{{ID: "session-1", WorkingDirectory: "/projects/a-gent"}}

	updatedModel, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	navigationMessage := command()
	_, quitCommand := updatedModel.(Model).Update(navigationMessage)

	if navigator.session.ID != "session-1" {
		t.Fatalf("navigated session = %q, want %q", navigator.session.ID, "session-1")
	}
	if quitCommand == nil {
		t.Fatal("successful navigation does not close the dashboard")
	}
}

func TestEnterDoesNothingWhenNavigationIsUnsupported(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{ID: "session-1", WorkingDirectory: "/projects/a-gent"}}

	_, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("unsupported navigation returned a command")
	}
	if strings.Contains(model.footerText(), "enter:") {
		t.Fatal("footer advertises navigation outside a supported terminal")
	}
}

func TestNavigationErrorUsesAVisibleTemporaryNotice(t *testing.T) {
	navigator := &fakeNavigator{err: errors.New("no tmux codex pane found for this project")}
	model := NewModel(nil, WithSessionNavigator(navigator))
	model.sessions = []agent.Session{{ID: "session-1", WorkingDirectory: "/projects/a-gent"}}

	updatedModel, navigationCommand := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	navigationMessage := navigationCommand()
	noticeModel, clearCommand := updatedModel.(Model).Update(navigationMessage)
	modelWithNotice := noticeModel.(Model)

	if clearCommand == nil {
		t.Fatal("navigation error does not schedule notice dismissal")
	}
	if footer := modelWithNotice.footerView(100); !strings.Contains(footer, "Could not open workspace") {
		t.Fatalf("navigation error is not visible in the footer: %q", footer)
	}

	expiredModel, _ := modelWithNotice.Update(noticeExpiredMessage{revision: modelWithNotice.noticeRevision})
	if footer := expiredModel.(Model).footerView(100); strings.Contains(footer, "Could not open workspace") {
		t.Fatalf("expired navigation error remains visible in the footer: %q", footer)
	}
}

func TestSessionSummary(t *testing.T) {
	summary := summarizeSessions([]agent.Session{
		{State: agent.StateRunning},
		{State: agent.StateWaiting},
		{State: agent.StateIdle},
		{State: agent.StateError},
		{State: agent.StateUnavailable},
	})

	if summary.total != 5 || summary.running != 1 || summary.waiting != 1 || summary.idle != 1 || summary.errors != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestOlderNoticeTimerDoesNotDismissNewError(t *testing.T) {
	model := NewModel(nil)
	firstModel, _ := model.Update(sessionNavigationMessage{err: errors.New("first error")})
	firstNotice := firstModel.(Model)
	secondModel, _ := firstNotice.Update(sessionNavigationMessage{err: errors.New("second error")})
	updatedModel, _ := secondModel.(Model).Update(noticeExpiredMessage{revision: firstNotice.noticeRevision})
	if !strings.Contains(updatedModel.(Model).notice, "second error") {
		t.Fatal("old timer dismissed the newer notice")
	}
}

func TestNavigationNoticeRemovesTerminalControls(t *testing.T) {
	model := NewModel(nil)
	updatedModel, _ := model.Update(sessionNavigationMessage{err: errors.New("bad\npath\x1b[31mred\x1b[0m\x1b]52;c;Y2xpcGJvYXJk\a")})
	notice := updatedModel.(Model).notice
	if strings.ContainsAny(notice, "\x1b\n\r\a") || strings.Contains(notice, "Y2xpcGJvYXJk") {
		t.Fatalf("unsafe control text remained in notice: %q", notice)
	}
	if !strings.Contains(notice, "bad pathred") {
		t.Fatalf("readable error details were lost: %q", notice)
	}
}

func TestEnterDoesNotNavigateSidebarOrEmptyTable(t *testing.T) {
	model := NewModel(nil, WithSessionNavigator(&fakeNavigator{}))
	if _, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter}); command != nil {
		t.Fatal("empty table attempted navigation")
	}
	model.sessions = []agent.Session{{ID: "main"}}
	model.sidebarFocus = true
	if _, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter}); command != nil {
		t.Fatal("sidebar attempted navigation")
	}
}

func TestSessionListCapsVisibleRowsAndScrolls(t *testing.T) {
	sessions := make([]agent.Session, maximumSessionRows+2)
	model := NewModel(nil)
	model.sessions = sessions
	model.height = 80
	model.resizeTable()

	if model.table.Height() != maximumSessionRows {
		t.Fatalf("table height = %d, want %d", model.table.Height(), maximumSessionRows)
	}

	model.table.SetCursor(maximumSessionRows)
	start, end := model.visibleSessionRange()
	if start != 1 || end != maximumSessionRows+1 {
		t.Fatalf("visible range = %d-%d, want 1-%d", start, end, maximumSessionRows+1)
	}
}

func TestSessionListRendersEmptyRowPlaceholders(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{Name: "First session"},
		{Name: "Second session"},
		{Name: "Third session"},
	}
	model.height = 80
	model.resizeTable()

	if model.table.Height() != maximumSessionRows {
		t.Fatalf("table height = %d, want %d", model.table.Height(), maximumSessionRows)
	}

	tableView := model.sessionTableView()
	if got, want := strings.Count(tableView, "\n")+1, maximumSessionRows+2; got != want {
		t.Fatalf("rendered table rows = %d, want %d including header", got, want)
	}
}

func TestEmptySessionListUsesAPlaceholderRow(t *testing.T) {
	model := NewModel(nil)
	model.height = 80
	model.resizeTable()

	tableView := model.sessionTableView()
	if !strings.Contains(tableView, "No live sessions found.") {
		t.Fatal("empty session list does not explain that no sessions were found")
	}
	if got, want := strings.Count(tableView, "\n")+1, maximumSessionRows+2; got != want {
		t.Fatalf("rendered table rows = %d, want %d including header", got, want)
	}
}

func TestSessionTableSeparatesTitleHeaderAndRows(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{Name: "Example", Provider: "codex", State: agent.StateRunning}}
	tableView := model.sessionTableView()
	if !strings.Contains(tableView, "Status") || !strings.Contains(tableView, "\n\n") || !strings.Contains(tableView, "› codex") {
		t.Fatalf("table header and body do not have visual separation:\n%s", tableView)
	}
	view := model.View()
	titleLine, headerLine := -1, -1
	for lineNumber, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "SESSIONS (1-1 of 1)") {
			titleLine = lineNumber
		}
		if strings.Contains(line, "Agent") && strings.Contains(line, "Status") {
			headerLine = lineNumber
		}
	}
	if titleLine < 0 || headerLine-titleLine < 2 {
		t.Fatalf("session title and table header lack vertical spacing (lines %d and %d):\n%s", titleLine, headerLine, view)
	}
}

func TestSelectionMarkerRemainsVisibleWhenSidebarHasFocus(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{Name: "Example", Provider: "codex", State: agent.StateRunning}}
	model.sidebarFocus = true
	row := model.sessionRowView(0, model.table.Columns())
	if !strings.Contains(row, "› codex") {
		t.Fatalf("selected row lacks its marker while sidebar has focus: %q", row)
	}
}

func TestSelectionCursorKeepsAgentNamesAligned(t *testing.T) {
	for _, width := range []int{minimumTableWidth, defaultTableWidth, 110} {
		model := NewModel(nil)
		model.sessions = []agent.Session{
			{Provider: "claude", Name: "First", State: agent.StateRunning},
			{Provider: "claude", Name: "Second", State: agent.StateIdle},
		}
		columns := tableColumns(width)
		model.table.SetColumns(columns)
		model.updateTableRows()

		selected := ansi.Strip(model.sessionRowView(0, columns))
		model.table.SetCursor(1)
		unselected := ansi.Strip(model.sessionRowView(0, columns))
		if !strings.HasPrefix(selected, "› claude") || !strings.HasPrefix(unselected, "  claude") {
			t.Fatalf("width %d: agent name shifted or truncated: selected %q, unselected %q", width, selected, unselected)
		}
		if []rune(selected)[0] != '›' || string([]rune(selected)[1:]) != string([]rune(unselected)[1:]) {
			t.Fatalf("width %d: selection changed content outside the cursor column", width)
		}
		wantWidth := width - panelStyle.GetHorizontalFrameSize()
		for _, row := range []string{selected, unselected, model.emptySessionRowView(columns, false), strings.Split(model.sessionTableView(), "\n")[0]} {
			if got := lipgloss.Width(row); got != wantWidth {
				t.Fatalf("width %d: rendered row width = %d, want %d", width, got, wantWidth)
			}
		}
	}
}

func TestPopupHeightFitsTheReservedSessionRows(t *testing.T) {
	model := NewModel(nil, WithSessionNavigator(&fakeNavigator{}))
	model.sessions = []agent.Session{{
		Name:             "Example session",
		Provider:         "codex",
		WorkingDirectory: "/projects/a-gent",
		State:            agent.StateIdle,
	}}

	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: PopupContentHeight})
	view := updatedModel.(Model).View()
	if got := lipgloss.Height(view); got != PopupContentHeight {
		t.Fatalf("popup content height = %d, want %d", got, PopupContentHeight)
	}
	if PopupHeight-PopupContentHeight != 2 {
		t.Fatalf("popup border height = %d, want 2", PopupHeight-PopupContentHeight)
	}
}

func TestSidebarRendersViewsAndProjects(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{WorkingDirectory: "/projects/a-gent", State: agent.StateRunning},
		{WorkingDirectory: "/projects/a-gent", State: agent.StateIdle},
		{WorkingDirectory: "/projects/other", State: agent.StateWaiting},
	}

	sidebar := model.renderSidebar(summarizeSessions(model.sessions))
	for _, expected := range []string{"VIEWS", "Attention", "Active", "Recent", "All", "PROJECTS", "a-gent", "other"} {
		if !strings.Contains(sidebar, expected) {
			t.Errorf("sidebar does not contain %q:\n%s", expected, sidebar)
		}
	}
	if !strings.Contains(sidebar, "● a-gent           2") {
		t.Fatalf("sidebar does not show the a-gent agent count:\n%s", sidebar)
	}
}

func TestViewsFilterSessions(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "running", State: agent.StateRunning},
		{ID: "waiting", State: agent.StateWaiting},
		{ID: "failed", State: agent.StateError},
		{ID: "finished", State: agent.StateIdle},
	}

	testCases := []struct {
		view    sidebarView
		wantIDs []string
	}{
		{view: attentionView, wantIDs: []string{"failed", "waiting"}},
		{view: activeView, wantIDs: []string{"running"}},
		{view: recentView, wantIDs: []string{"finished"}},
		{view: allView, wantIDs: []string{"failed", "finished", "running", "waiting"}},
	}

	for _, testCase := range testCases {
		t.Run(string(testCase.view), func(t *testing.T) {
			model.selectedView = testCase.view
			model.selectedProject = ""
			sessions := model.filteredSessions()
			if len(sessions) != len(testCase.wantIDs) {
				t.Fatalf("filtered sessions = %d, want %d", len(sessions), len(testCase.wantIDs))
			}
			for index, session := range sessions {
				if session.ID != testCase.wantIDs[index] {
					t.Errorf("session %d = %q, want %q", index, session.ID, testCase.wantIDs[index])
				}
			}
		})
	}
}

func TestSelectingProjectFiltersTableAndDetails(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "a-gent", Name: "a-gent session", WorkingDirectory: "/projects/a-gent", State: agent.StateRunning},
		{ID: "other", Name: "other session", WorkingDirectory: "/projects/other", State: agent.StateIdle},
	}
	model.sidebarCursor = len(sidebarViews())
	model.moveSidebarCursor(1)

	if got := len(model.table.Rows()); got != 1 {
		t.Fatalf("table rows = %d, want 1", got)
	}
	if model.selectedProject != "/projects/other" {
		t.Fatalf("selected project = %q, want /projects/other", model.selectedProject)
	}
	if detail := model.detailView(); !strings.Contains(detail, "other session") {
		t.Fatalf("details do not use the project-filtered session:\n%s", detail)
	}
}
