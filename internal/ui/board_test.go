package ui

import (
	"fmt"
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

	// Card traversal follows status columns while the shared table keeps its sort.
	model.table.SetCursor(1)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
	for _, size := range [][2]int{{120, 28}, {100, 28}, {80, 24}, {64, 20}, {40, 16}, {24, 10}} {
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
