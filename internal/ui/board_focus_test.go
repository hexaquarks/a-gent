package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestBoardFocusUsesSelectedSessionPreview(t *testing.T) {
	model := referenceDashboard()
	model.boardVisible = true
	selected, _ := model.selectedSession()
	key := sessionIdentity{provider: selected.Provider, id: selected.ID}
	other := sessionIdentity{provider: "claude", id: selected.ID}
	model.previewKey = other
	model.previewCache[other] = previewEntry{
		loaded: true, checked: time.Now(),
		edit: &agent.Edit{Filename: "other-session.go", Diff: "+unrelated"},
	}
	model.previewCache[key] = previewEntry{
		loaded: true, checked: time.Now(),
		edit: &agent.Edit{Filename: "selected.go", Diff: "@@ change\n-old\n+new\n+extra"},
	}

	view := ansi.Strip(model.boardFocusView(160))
	for _, text := range []string{
		"FOCUS", selected.Name, "codex", "swiftyprompt", "Idle", "selected.go", "+2", "−1", "v details / diff",
	} {
		if !strings.Contains(view, text) {
			t.Errorf("selected focus is missing %q:\n%s", text, view)
		}
	}
	if strings.Contains(view, "other-session.go") {
		t.Fatalf("focus used another provider's preview:\n%s", view)
	}

	delete(model.previewCache, key)
	view = ansi.Strip(model.boardFocusView(160))
	if strings.Contains(view, "other-session.go") || strings.Contains(view, "selected.go") {
		t.Fatalf("missing selected preview fell back to another session:\n%s", view)
	}
}

func TestBoardFocusSwitchesFromActivityToEditWhenIdle(t *testing.T) {
	model := referenceDashboard()
	model.boardVisible = true
	model.sessions = []agent.Session{{
		ID:       "one",
		Provider: "codex",
		Name:     "Selected",
		State:    agent.StateRunning,
	}}
	model.updateTableRows()
	key := sessionIdentity{provider: "codex", id: "one"}
	model.previewCache = map[sessionIdentity]previewEntry{key: {
		loaded: true, checked: time.Now(),
		edit:     &agent.Edit{Filename: "retained.go", Diff: "+kept"},
		activity: &agent.Activity{Label: "Assistant", Text: "Checking the implementation", At: time.Now()},
	}}

	view := ansi.Strip(model.boardFocusView(160))
	if !strings.Contains(view, "Checking the implementation") || strings.Contains(view, "retained.go") {
		t.Fatalf("running session did not prioritize live activity:\n%s", view)
	}

	model.sessions[0].State = agent.StateIdle
	model.updateTableRows()
	view = ansi.Strip(model.boardFocusView(160))
	if !strings.Contains(view, "retained.go") || strings.Contains(view, "Checking the implementation") {
		t.Fatalf("idle session did not show its retained edit:\n%s", view)
	}
}

func TestBoardFocusMarksRetainedPreviewStale(t *testing.T) {
	for _, reason := range []string{"failed refresh", "stale session", "old cache"} {
		t.Run(reason, func(t *testing.T) {
			model := referenceDashboard()
			selected, _ := model.selectedSession()
			key := sessionIdentity{provider: selected.Provider, id: selected.ID}
			cached := model.previewCache[key]

			switch reason {
			case "failed refresh":
				cached.err = errors.New("offline")
			case "stale session":
				for index := range model.sessions {
					model.sessions[index].Stale = true
				}
			case "old cache":
				cached.checked = time.Now().Add(-time.Minute)
			}
			model.previewCache[key] = cached

			view := ansi.Strip(model.boardFocusView(160))
			if !strings.Contains(view, cached.edit.Filename) || !strings.Contains(strings.ToLower(view), "stale") {
				t.Fatalf("retained preview lost its stale signal:\n%s", view)
			}
		})
	}
}

func TestBoardFocusEmptyPreviewShowsFailure(t *testing.T) {
	model := referenceDashboard()
	selected, _ := model.selectedSession()
	model.previewSources = map[string]agent.PreviewSource{selected.Provider: &previewSource{}}
	key := sessionIdentity{provider: selected.Provider, id: selected.ID}
	model.previewCache[key] = previewEntry{loaded: true, checked: time.Now(), err: errors.New("offline")}

	view := ansi.Strip(model.boardFocusView(160))
	if !strings.Contains(view, "Preview unavailable") || !strings.Contains(view, "v details / diff") {
		t.Fatalf("focus hid the preview failure or expansion shortcut:\n%s", view)
	}
}

func TestBoardFocusCompactsMultilineActivity(t *testing.T) {
	for _, width := range []int{120, 40} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			model := referenceDashboard()
			updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 28})
			model = updated.(Model)
			selected, _ := model.selectedSession()
			key := sessionIdentity{provider: selected.Provider, id: selected.ID}
			model.previewCache[key] = previewEntry{
				loaded: true, checked: time.Now(),
				activity: &agent.Activity{Label: "Assistant", Text: "First line\n\x1b[2JSecond line", At: time.Now()},
			}

			view := model.boardFocusView(width)
			if lipgloss.Width(view) > width || lipgloss.Height(view) != model.boardFocusHeight() {
				t.Fatal("multiline activity overflowed the focus strip")
			}
			if !strings.Contains(ansi.Strip(view), "v details / diff") {
				t.Fatal("multiline activity displaced the details shortcut")
			}
		})
	}
}

func TestBoardFocusFitsTerminalAndReturnsFromExpandedPreview(t *testing.T) {
	for _, size := range [][2]int{{120, 28}, {80, 24}, {40, 16}, {24, 10}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			model := referenceDashboard()
			model.boardVisible = true
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model = updated.(Model)
			selected, _ := model.selectedSession()
			wantFocusHeight := 3
			if size[0] == 120 {
				wantFocusHeight = 2
			} else if size[1] == 10 {
				wantFocusHeight = 1
			}
			if model.boardFocusHeight() != wantFocusHeight {
				t.Fatalf("focus height = %d, want %d", model.boardFocusHeight(), wantFocusHeight)
			}
			view := model.View()
			if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
				t.Fatalf("board size = %dx%d, want %dx%d", lipgloss.Width(view), lipgloss.Height(view), size[0], size[1])
			}
			if strings.Contains(ansi.Strip(view), "SELECTED SESSION") {
				t.Fatal("board still reserves the full selected-session panel")
			}
			shortcut := "v details / diff"
			if size[0] < 32 {
				shortcut = "v expand"
			}
			if !strings.Contains(ansi.Strip(view), shortcut) {
				t.Fatalf("board lost its expansion shortcut:\n%s", ansi.Strip(view))
			}
			if size[1] >= 24 && lipgloss.Height(model.boardView()) <= model.table.Height()+1 {
				t.Fatal("board did not gain space from the reduced details")
			}

			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
			model = updated.(Model)
			if !model.previewExpanded {
				t.Fatal("details shortcut did not expand the preview")
			}
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(Model)
			current, _ := model.selectedSession()
			if model.previewExpanded || !model.boardVisible ||
				current.ID != selected.ID || current.Provider != selected.Provider {
				t.Fatal("returning from details lost the selected board card")
			}
			if model.View() != view {
				t.Fatal("returning from details changed the board layout")
			}
		})
	}
}
