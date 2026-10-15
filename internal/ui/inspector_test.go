package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestInspectorShowsActivityAndEditTogether(t *testing.T) {
	model := referenceDashboard()
	selected, _ := model.selectedSession()
	key := sessionIdentity{provider: selected.Provider, id: selected.ID}
	cached := model.previewCache[key]
	cached.activity = &agent.Activity{Label: "Testing", Text: "Keyboard navigation passes.", At: time.Now()}
	model.previewCache[key] = cached
	for _, state := range []agent.State{agent.StateRunning, agent.StateWaiting, agent.StateIdle} {
		model.sessions[2].State = state
		view := ansi.Strip(model.detailWithPreview())
		for _, text := range []string{
			"ACTIVITY", "Testing", "Keyboard navigation passes.", "LATEST EDIT", "internal/navigation.go",
			"-    navigator.Open(cwd)", "+    navigator.Navigate(",
		} {
			if !strings.Contains(view, text) {
				t.Fatalf("%s inspector omitted %q:\n%s", state, text, view)
			}
		}
	}
	cached.err = tea.ErrProgramKilled
	model.previewCache[key] = cached
	view := ansi.Strip(model.detailWithPreview())
	if strings.Count(view, "stale") != 2 {
		t.Fatalf("both cached records should identify stale data:\n%s", view)
	}
}

func TestInspectorCollapseReclaimsSpaceAndKeepsSelection(t *testing.T) {
	for _, board := range []bool{false, true} {
		model := referenceDashboard()
		model.boardVisible = board
		selected, _ := model.selectedSession()
		expandedHeight := model.table.Height()
		boardHeight := model.boardHeight()
		updated, _ := model.Update(runeKey('i'))
		model = updated.(Model)
		current, _ := model.selectedSession()
		if !model.inspectorCollapsed || selected.ID != current.ID || model.table.Height() <= expandedHeight {
			t.Fatal("collapsing inspector did not reclaim rows while keeping selection")
		}
		if board && model.boardHeight() <= boardHeight {
			t.Fatal("collapsing inspector did not reclaim board rows")
		}
		updated, _ = model.Update(runeKey('v'))
		model = updated.(Model)
		if !model.previewExpanded {
			t.Fatal("collapsed inspector could not expand")
		}
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model = updated.(Model)
		if !model.inspectorCollapsed || model.previewExpanded {
			t.Fatal("returning from expansion changed collapsed state")
		}
		updated, _ = model.Update(runeKey('i'))
		model = updated.(Model)
		if model.inspectorCollapsed || model.table.Height() != expandedHeight {
			t.Fatal("reopening inspector did not restore rows")
		}
	}
}

func TestInspectorFitsResizedTerminals(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {40, 15}, {64, 21}, {80, 24}, {120, 29}, {160, 45}} {
		for _, collapsed := range []bool{false, true} {
			model := referenceDashboard()
			model.inspectorCollapsed = collapsed
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model = updated.(Model)
			for _, board := range []bool{false, true} {
				model.boardVisible = board
				view := model.View()
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%v collapsed=%t board=%t overflows", size, collapsed, board)
				}
			}
		}
	}
}

func TestExpandedInspectorSwitchesBetweenActivityAndScrollableEdit(t *testing.T) {
	for _, state := range []agent.State{agent.StateRunning, agent.StateWaiting, agent.StateIdle} {
		t.Run(string(state), func(t *testing.T) {
			model := referenceDashboard()
			model.sessions[2].State = state
			selected, _ := model.selectedSession()
			key := sessionIdentity{provider: selected.Provider, id: selected.ID}
			diff := []string{"@@ change", "+DIFF_START"}
			for index := 0; index < 40; index++ {
				diff = append(diff, "+more changed code")
			}
			diff = append(diff, "+DIFF_END")
			cached := model.previewCache[key]
			cached.edit.Diff = strings.Join(diff, "\n")
			cached.activity = &agent.Activity{Label: "Testing", Text: "ACTIVITY_OUTPUT", At: time.Now()}
			model.previewCache[key] = cached

			updated, _ := model.Update(runeKey('v'))
			model = updated.(Model)
			if state == agent.StateIdle {
				if !model.previewShowsEdit {
					t.Fatal("idle session should initially expand its latest edit")
				}
				updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
				model = updated.(Model)
			}
			activity := ansi.Strip(model.View())
			for _, text := range []string{"ACTIVITY_OUTPUT", "Tab switch", selected.ID} {
				if !strings.Contains(activity, text) {
					t.Fatalf("expanded activity omitted %q:\n%s", text, activity)
				}
			}

			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
			model = updated.(Model)
			edit := ansi.Strip(model.View())
			if !strings.Contains(edit, "DIFF_START") || strings.Contains(edit, "ACTIVITY_OUTPUT") {
				t.Fatalf("Tab did not select latest edit:\n%s", edit)
			}
			for index := 0; index < len(diff); index++ {
				updated, _ = model.Update(runeKey('j'))
				model = updated.(Model)
			}
			if !strings.Contains(ansi.Strip(model.View()), "DIFF_END") {
				t.Fatal("expanded edit could not scroll to its last changed line")
			}
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
			model = updated.(Model)
			if model.previewScroll != 0 || !strings.Contains(model.View(), "ACTIVITY_OUTPUT") {
				t.Fatal("switching back to activity did not reset scrolling")
			}
		})
	}
}

func TestInspectorDistinguishesEmptyFilterFromNoLiveSessions(t *testing.T) {
	model := referenceDashboard()
	model.selectedProject = "/missing"
	model.updateTableRows()
	view := ansi.Strip(model.detailWithPreview())
	if !strings.Contains(view, "No sessions match this filter.") || strings.Contains(view, "No live sessions") {
		t.Fatalf("empty filter was reported as an empty dashboard:\n%s", view)
	}
	model.sessions = nil
	model.updateTableRows()
	if !strings.Contains(ansi.Strip(model.detailWithPreview()), "No live sessions found.") {
		t.Fatal("empty dashboard lost its no-live-sessions message")
	}
}
