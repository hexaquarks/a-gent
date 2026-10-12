package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestBoardSelectionAndLiveStatus(t *testing.T) {
	model := referenceDashboard()
	selected, _ := model.selectedSession()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	model = updated.(Model)
	if current, _ := model.selectedSession(); current.ID != selected.ID || !model.boardVisible {
		t.Fatal("switching layout changed the selected session")
	}

	// Column navigation keeps the existing table sort and selected identity.
	model.table.SetCursor(1)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = updated.(Model)
	if current, _ := model.selectedSession(); current.Provider != "claude" {
		t.Fatalf("expected the running card after the waiting card, got %+v", current)
	}

	model.applyProviderUpdate(polling.Update{Provider: "claude", Sessions: []agent.Session{
		{ID: "claude-session", Name: "Changed status", State: agent.StateWaiting},
	}})
	current, _ := model.selectedSession()
	if current.ID != "claude-session" || boardColumnIndex(current) != 0 {
		t.Fatal("status update did not move the selected card with its session")
	}
	model.selectedProvider = "claude"
	model.updateTableRows()
	view := ansi.Strip(model.boardView())
	if !strings.Contains(view, "Changed status") || strings.Contains(view, "Restore response") {
		t.Fatalf("board did not apply the provider filter:\n%s", view)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	model = updated.(Model)
	if current, _ := model.selectedSession(); current.ID != "claude-session" || model.boardVisible {
		t.Fatal("returning to List lost selection")
	}
}

func TestBoardGroupsAttentionStates(t *testing.T) {
	for _, session := range []agent.Session{
		{State: agent.StateWaiting}, {State: agent.StateError},
		{State: agent.StateUnavailable}, {State: agent.StateRunning, Stale: true}, {},
	} {
		if boardColumnIndex(session) != 0 {
			t.Errorf("session requiring attention appeared outside Needs You: %+v", session)
		}
	}
}

func TestBoardLayoutAndScrolling(t *testing.T) {
	for _, size := range [][2]int{{120, 28}, {100, 28}, {80, 24}, {64, 20}, {40, 16}, {24, 10}, {24, 11}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			model := referenceDashboard()
			model.boardVisible = true
			model.sessions = nil
			model.sort = sessionSort{column: "Last active", descending: true}
			for index := 0; index < 20; index++ {
				model.sessions = append(model.sessions, agent.Session{
					ID:       fmt.Sprintf("%02d", index),
					Provider: "codex",
					State:    agent.StateIdle,
					Name:     fmt.Sprintf("Card %02d", index),
				})
			}
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model = updated.(Model)
			model.table.SetCursor(19)
			view := model.View()
			if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
				t.Fatalf("board dimensions changed: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
			}
			if !strings.Contains(ansi.Strip(view), "Card 19") {
				t.Fatal("scrolling hid the selected card")
			}
			model.moveBoardCursor(1)
			if model.table.Cursor() != 19 {
				t.Fatal("moving at the last card should keep selection")
			}
		})
	}
}

func TestBoardReferenceCapture(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	model := referenceDashboard()
	model.boardVisible = true
	view := model.View()
	for _, text := range []string{"NEEDS YOU · 1", "RUNNING · 1", "IDLE · 4", " List  Board ", "FOCUS"} {
		if !strings.Contains(ansi.Strip(view), text) {
			t.Errorf("board is missing %q", text)
		}
	}
	writeReferenceCapture(t, view, ansi.Strip(view))
}

func TestBoardCardFramesAndMetadata(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	model := referenceDashboard()
	session := agent.Session{
		ID:               "card",
		Provider:         "codex",
		Name:             "Readable title",
		State:            agent.StateRunning,
		WorkingDirectory: "/projects/a-gent",
	}
	for _, selected := range []bool{false, true} {
		view := model.boardCardView(session, selected, 36)
		if lipgloss.Width(view) != 36 || lipgloss.Height(view) != 5 {
			t.Fatalf("card size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
		}
		if !strings.Contains(strings.Split(view, "\n")[0], "48;2;12;17;18") {
			t.Fatal("card border lost the page background separating neighboring cards")
		}
		lines := strings.Split(ansi.Strip(view), "\n")
		if !strings.Contains(lines[1], "Readable title") || !strings.Contains(lines[2], "codex · a-gent") ||
			!strings.HasSuffix(lines[2], "—│") || !strings.Contains(lines[3], "Running") {
			t.Fatalf("card content or aligned age changed:\n%s", ansi.Strip(view))
		}
		if !strings.Contains(view, "38;2;163;239;120") {
			t.Fatal("card lost its green running edge/status")
		}
		if regexp.MustCompile(`38;2;89;223;240[^m]*m┌`).MatchString(view) != selected {
			t.Fatal("cyan outline does not follow selection")
		}
		background := "48;2;32;43;44"
		if selected {
			background = "48;2;24;55;64"
		}
		if !strings.Contains(view, background) {
			t.Fatal("card lost its shaded background")
		}
	}
}

func TestBoardColumnDividersAndSelectedCardVisibility(t *testing.T) {
	model := referenceDashboard()
	view := ansi.Strip(model.boardView())
	rows := strings.Split(view, "\n")
	if strings.Count(rows[0], "│") != 2 || strings.Count(rows[1], "─") < 60 {
		t.Fatalf("board lost the column dividers or heading underlines:\n%s", view)
	}
	for _, height := range []int{1, 2, 3, 6, 7, 12} {
		column := boardColumn{title: "IDLE", color: colorSecondary, indexes: []int{0, 1, 2}}
		sessions := []agent.Session{{Name: "First"}, {Name: "Second"}, {Name: "Selected"}}
		model.table.SetCursor(2)
		view := model.boardColumnView(column, sessions, 24, height)
		if !strings.Contains(ansi.Strip(view), "Selected") || lipgloss.Height(view) != height ||
			lipgloss.Width(view) > 24 {
			t.Fatalf("height %d hid the selected title or exceeded its bounds:\n%s", height, ansi.Strip(view))
		}
	}
}

func TestBoardStandardPopupFitsFourCards(t *testing.T) {
	model := referenceDashboard()
	column := boardColumn{title: "IDLE", color: colorSecondary, indexes: []int{0, 1, 2, 3}}
	sessions := []agent.Session{{Name: "One"}, {Name: "Two"}, {Name: "Three"}, {Name: "Four"}}
	for _, selected := range []int{0, 1, 2, 3} {
		model.table.SetCursor(selected)
		view := model.boardColumnView(column, sessions, 28, model.boardHeight())
		lines := strings.Split(ansi.Strip(view), "\n")
		for index, session := range sessions {
			if !strings.Contains(lines[2+4*index], session.Name) {
				t.Fatalf("selection moved or hid card %s:\n%s", session.Name, ansi.Strip(view))
			}
		}
		if !strings.HasPrefix(lines[1+4*selected], "┌") || !strings.HasPrefix(lines[5+4*selected], "└") {
			t.Fatal("selection clipped a card border")
		}
		if lipgloss.Height(view) != model.boardHeight() {
			t.Fatal("cards exceeded the available height")
		}
	}
}

func TestBoardCardsShareBorderRowWithoutClippingSelection(t *testing.T) {
	model := referenceDashboard()
	column := boardColumn{title: "IDLE", color: colorSecondary, indexes: []int{0, 1, 2}}
	sessions := []agent.Session{{Name: "First"}, {Name: "Middle"}, {Name: "Last"}}
	for _, selected := range []int{0, 1, 2} {
		model.table.SetCursor(selected)
		view := model.boardColumnView(column, sessions, 24, 15)
		lines := strings.Split(ansi.Strip(view), "\n")
		firstBorder := 1
		for index, session := range sessions {
			if !strings.Contains(lines[firstBorder+1+4*index], session.Name) {
				t.Fatalf("card spacing changed or hid a title:\n%s", ansi.Strip(view))
			}
		}
		if !strings.HasPrefix(lines[firstBorder+4*selected], "┌") || !strings.HasPrefix(lines[firstBorder+4+4*selected], "└") {
			t.Fatalf("shared border row clipped the selected outline:\n%s", ansi.Strip(view))
		}
	}
}

func TestBoardCardsRemoveSelectionArrowAndReservedSpace(t *testing.T) {
	model := referenceDashboard()
	session := agent.Session{ID: "card", Provider: "codex", Name: "Title", State: agent.StateIdle}
	for _, unread := range []bool{false, true} {
		model.unreadSessions = map[sessionIdentity]bool{{provider: session.Provider, id: session.ID}: unread}
		for _, selected := range []bool{false, true} {
			lines := strings.Split(ansi.Strip(model.boardCardView(session, selected, 24)), "\n")
			prefix := "│▌"
			if unread {
				prefix += "● "
			}
			if strings.Contains(strings.Join(lines, "\n"), "›") || !strings.HasPrefix(lines[1], prefix+"Title") ||
				!strings.HasPrefix(lines[2], "│▌codex") {
				t.Fatalf("board kept its arrow or reserved left space:\n%s", strings.Join(lines, "\n"))
			}
		}
	}
}

func TestBoardColumnShowsHiddenCardCounts(t *testing.T) {
	model := referenceDashboard()
	column := boardColumn{title: "IDLE", color: colorSecondary}
	var sessions []agent.Session
	for index := range 10 {
		column.indexes = append(column.indexes, index)
		sessions = append(sessions, agent.Session{Name: fmt.Sprintf("Card %d", index)})
	}
	model.sessions = sessions
	model.updateTableRows()
	for _, selected := range []int{0, 5, 9} {
		model.table.SetCursor(selected)
		view := model.boardColumnView(column, sessions, 28, 18)
		plain := ansi.Strip(view)
		start := max(0, selected-3)
		end := min(10, start+4)
		for _, indicator := range []struct {
			count int
			arrow string
		}{{start, "↑"}, {10 - end, "↓"}} {
			heading := strings.Split(plain, "\n")[0]
			count := fmt.Sprintf("%s%d", indicator.arrow, indicator.count)
			if indicator.count > 0 && !strings.Contains(heading, count) {
				t.Fatalf("missing hidden-card count:\n%s", plain)
			}
		}
		if !strings.Contains(plain, fmt.Sprintf("Card %d", selected)) || lipgloss.Height(view) != 18 {
			t.Fatalf("column clipped selection or exceeded its height:\n%s", plain)
		}
	}
}
