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

func sidebarViewIndex(view sidebarView) int {
	for index, candidate := range sidebarViews() {
		if candidate == view {
			return index
		}
	}
	panic("unknown sidebar view")
}

func TestAttentionFiltersMatchTheirCounts(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{
		{ID: "same", Provider: "codex", State: agent.StateWaiting},
		{ID: "same", Provider: "claude", State: agent.StateIdle},
		{ID: "error", Provider: "codex", State: agent.StateError},
		{ID: "unavailable", Provider: "codex", State: agent.StateUnavailable},
		{ID: "stale", Provider: "codex", State: agent.StateRunning, Stale: true},
		{ID: "running", Provider: "codex", State: agent.StateRunning},
		{ID: "unknown", Provider: "codex"},
	}
	model.unreadSessions = map[sessionIdentity]bool{{provider: "claude", id: "same"}: true}

	for _, test := range []struct {
		view sidebarView
		want int
	}{
		{view: needsYouView, want: 5},
		{view: unseenView, want: 1},
	} {
		t.Run(string(test.view), func(t *testing.T) {
			model.selectedView = test.view
			filtered := model.filteredSessions()
			count := model.viewCount(test.view, summarizeSessions(model.sessions))
			if len(filtered) != test.want || count != test.want {
				t.Fatalf("filter has %d sessions, count %d, want %d", len(filtered), count, test.want)
			}
			if test.view == unseenView && filtered[0].Provider != "claude" {
				t.Fatal("Unseen confused sessions with the same ID from different providers")
			}
		})
	}
}

func TestUnseenAcknowledgementOpensOriginalSessionAndRefreshesRows(t *testing.T) {
	for _, board := range []bool{false, true} {
		t.Run(fmt.Sprint(board), func(t *testing.T) {
			navigator := &fakeNavigator{}
			model := NewModel(nil, WithSessionNavigator(navigator))
			model.boardVisible = board
			model.selectedView = unseenView
			model.sessions = []agent.Session{
				{ID: "first", Provider: "codex", Name: "Alpha", State: agent.StateIdle},
				{ID: "second", Provider: "codex", Name: "Beta", State: agent.StateIdle},
			}
			model.unreadSessions = map[sessionIdentity]bool{
				{provider: "codex", id: "first"}:  true,
				{provider: "codex", id: "second"}: true,
			}
			model.updateTableRows()
			selected, _ := model.selectedSession()

			updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(Model)
			if len(model.table.Rows()) != 1 || model.tableSessionIDs[0].id == selected.ID {
				t.Fatal("acknowledgement did not immediately remove the original session")
			}
			if command == nil {
				t.Fatal("Enter did not navigate")
			}
			updated, _ = model.Update(command())
			model = updated.(Model)
			if navigator.session.ID != selected.ID {
				t.Fatalf("opened %q instead of original session %q", navigator.session.ID, selected.ID)
			}
			if count := model.viewCount(unseenView, summarizeSessions(model.sessions)); count != 1 {
				t.Fatalf("Unseen count = %d after acknowledgement", count)
			}
		})
	}
}

func TestProjectWaitingBadgeReservesStableColumns(t *testing.T) {
	for _, count := range []int{1, 12, 1000} {
		model := NewModel(nil)
		for index := 0; index < count; index++ {
			model.sessions = append(model.sessions, agent.Session{
				ID: fmt.Sprint(index), Provider: "codex", State: agent.StateRunning,
				WorkingDirectory: "/projects/長いプロジェクト名",
			})
		}
		// A different project keeps the shared badge column present throughout.
		for index := 0; index < count; index++ {
			model.sessions = append(model.sessions, agent.Session{
				ID: fmt.Sprintf("other-%d", index), Provider: "codex", State: agent.StateWaiting,
				WorkingDirectory: "/projects/other",
			})
		}
		item := sidebarItem{label: "長いプロジェクト名", project: model.sessions[0].WorkingDirectory}
		model.pinnedProjects = map[string]bool{item.project: true}
		before := ansi.Strip(model.projectItemView(item, false, false))
		for index := range model.sessions[:count] {
			model.sessions[index].State = agent.StateWaiting
		}
		after := ansi.Strip(model.projectItemView(item, false, false))
		badge := fmt.Sprintf("!%d", count)
		if !strings.Contains(after, badge) {
			t.Fatalf("missing waiting badge %q: %q", badge, after)
		}
		if strings.Replace(after, badge, strings.Repeat(" ", len(badge)), 1) != before {
			t.Fatalf("badge moved the name, pin, or total: %q versus %q", before, after)
		}
		if lipgloss.Width(after) != model.sidebarWidth() {
			t.Fatalf("badge overflowed sidebar: %q", after)
		}
		for index := range model.sessions[:count] {
			model.sessions[index].Stale = true
		}
		if stale := ansi.Strip(model.projectItemView(item, false, false)); stale != before {
			t.Fatalf("stale waiting sessions still have an attention badge: %q", stale)
		}
	}
}

func TestAttentionFiltersFitNarrowDashboard(t *testing.T) {
	for _, width := range []int{24, 40, 62, 120} {
		model := referenceDashboard()
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 16})
		model = updated.(Model)
		view := model.View()
		if lipgloss.Width(view) != width || lipgloss.Height(view) != 16 {
			t.Fatalf("dashboard width %d rendered %dx%d", width, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestBoardAttentionFiltersSurfaceOwningParent(t *testing.T) {
	for _, view := range []sidebarView{needsYouView, unseenView} {
		t.Run(string(view), func(t *testing.T) {
			model := NewModel(nil)
			model.boardVisible = true
			model.selectedView = view
			model.sessions = []agent.Session{
				{ID: "root", Provider: "codex", State: agent.StateRunning, WorkingDirectory: "/one"},
				{ID: "child", ParentID: "root", Provider: "codex", State: agent.StateRunning, WorkingDirectory: "/one"},
				{ID: "nested", ParentID: "child", Provider: "codex", State: agent.StateWaiting, WorkingDirectory: "/one"},
				{ID: "root", Provider: "claude", State: agent.StateRunning, WorkingDirectory: "/two"},
				{ID: "orphan", ParentID: "missing", Provider: "claude", State: agent.StateWaiting},
				{ID: "cycle-a", ParentID: "cycle-b", Provider: "claude", State: agent.StateWaiting},
				{ID: "cycle-b", ParentID: "cycle-a", Provider: "claude", State: agent.StateWaiting},
			}
			model.unreadSessions = map[sessionIdentity]bool{
				{provider: "codex", id: "nested"}:   true,
				{provider: "claude", id: "orphan"}:  true,
				{provider: "claude", id: "cycle-a"}: true,
			}
			filtered := model.filteredSessions()
			if len(filtered) != 1 || filtered[0].ID != "root" || filtered[0].Provider != "codex" {
				t.Fatalf("Board did not surface the valid same-provider ancestor: %+v", filtered)
			}

			model.selectedProvider = "claude"
			if filtered := model.filteredSessions(); len(filtered) != 0 {
				t.Fatalf("attention from another provider leaked through filter: %+v", filtered)
			}
			model.selectedProvider = ""
			model.selectedProject = "/two"
			filtered = model.filteredSessions()
			if len(filtered) != 1 || filtered[0].WorkingDirectory != "/two" {
				t.Fatalf("owning card bypassed selected project: %+v", filtered)
			}

			model.selectedProject = ""
			model.selectedView = recentView
			if filtered := model.filteredSessions(); len(filtered) != 0 {
				t.Fatalf("attention aggregation changed Recent semantics: %+v", filtered)
			}
		})
	}
}

func TestBoardAcknowledgesOnlyOwningTree(t *testing.T) {
	for _, board := range []bool{false, true} {
		t.Run(fmt.Sprint(board), func(t *testing.T) {
			model := NewModel(nil)
			model.boardVisible = board
			model.sessions = []agent.Session{
				{ID: "root", Provider: "codex", Name: "Alpha", State: agent.StateIdle},
				{ID: "child", ParentID: "root", Provider: "codex", State: agent.StateIdle},
				{ID: "nested", ParentID: "child", Provider: "codex", State: agent.StateIdle},
				{ID: "other", Provider: "codex", Name: "Other", State: agent.StateIdle},
				{ID: "root", Provider: "claude", State: agent.StateIdle},
				{ID: "child", ParentID: "root", Provider: "claude", State: agent.StateIdle},
			}
			model.unreadSessions = make(map[sessionIdentity]bool)
			for _, session := range model.sessions {
				model.unreadSessions[sessionIdentity{provider: session.Provider, id: session.ID}] = true
			}
			rootIdentity := sessionIdentity{provider: "codex", id: "root"}
			delete(model.unreadSessions, rootIdentity)
			if !model.boardSessionUnread(model.sessions[0]) {
				t.Fatal("child-only unread group has no Board indicator")
			}
			model.updateTableRows()
			for index, identity := range model.tableSessionIDs {
				if identity == rootIdentity {
					model.table.SetCursor(index)
				}
			}
			model.markSelectedSessionRead()
			for _, session := range model.sessions {
				identity := sessionIdentity{provider: session.Provider, id: session.ID}
				wantUnread := session.Provider == "claude" || session.ID == "other" ||
					(!board && session.ParentID != "")
				if model.unreadSessions[identity] != wantUnread {
					t.Fatalf("session %+v unread = %v, want %v", identity, model.unreadSessions[identity], wantUnread)
				}
			}
			if board && model.boardSessionUnread(model.sessions[0]) {
				t.Fatal("Board group remains unread after opening its workspace")
			}
		})
	}
}

func TestSidebarReclaimsUnusedAttentionSpace(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{Provider: "codex", State: agent.StateIdle}}
	item := sidebarItem{label: "long-provider-x", provider: "codex"}
	withoutWaiting := ansi.Strip(model.sidebarItemView(item, 0, 1))
	if !strings.Contains(withoutWaiting, item.label) || !strings.HasSuffix(withoutWaiting, "1 ") {
		t.Fatalf("name or total lost space to an absent badge: %q", withoutWaiting)
	}
	if width := model.sidebarAttentionWidth(); width != 0 {
		t.Fatalf("absent attention badge reserves %d cells", width)
	}

	model.sessions[0].State = agent.StateWaiting
	withWaiting := ansi.Strip(model.sidebarItemView(item, 0, 1))
	if !strings.HasSuffix(withWaiting, "1    ") || !strings.Contains(withWaiting, "…") {
		t.Fatalf("waiting badges did not get a shared suffix: %q", withWaiting)
	}
	if lipgloss.Width(withoutWaiting) != lipgloss.Width(withWaiting) {
		t.Fatal("showing waiting badges changed the sidebar width")
	}

	model.sessions[0].Stale = true
	if stale := ansi.Strip(model.sidebarItemView(item, 0, 1)); stale != withoutWaiting {
		t.Fatalf("stale waiting session kept an empty badge column: %q", stale)
	}
}

func TestSidebarSeparatesMarkersFromFullWidthTotals(t *testing.T) {
	model := NewModel(nil)
	model.sessions = make([]agent.Session, 12)
	for _, marker := range []string{"★", indicatorDot} {
		row := ansi.Strip(model.renderSidebarRow(
			"Example", "", marker, "12", "", mainTextStyle, mutedStyle, mutedStyle,
		))
		if !strings.HasSuffix(row, marker+" 12 ") {
			t.Fatalf("marker touches the total or leaves a ghost suffix: %q", row)
		}
		if lipgloss.Width(row) != model.sidebarWidth() {
			t.Fatalf("row width changed: %q", row)
		}
	}

	model.sessions = make([]agent.Session, 1000)
	model.sessions[0].State = agent.StateWaiting
	if width := model.sidebarAttentionWidth(); width != len("!1") {
		t.Fatalf("badge width uses session total instead of actual waiting count: %d", width)
	}
}
