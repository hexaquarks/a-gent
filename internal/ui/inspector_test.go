package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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

func TestInspectorFitsResizedTerminals(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {40, 15}, {64, 21}, {80, 24}, {120, 29}, {160, 45}} {
		model := referenceDashboard()
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model = updated.(Model)
		for _, board := range []bool{false, true} {
			model.boardVisible = board
			view := model.View()
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%v board=%t overflows", size, board)
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

func TestInspectorDiffKeepsProviderLineNumbersAcrossHunks(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n" +
		"@@ -42,2 +42,3 @@\n-old\n+new\n+extra\n context\n" +
		"@@ -100 +101 @@\n-second old\n+second new"
	rows := inspectorDiffRows(diff)
	expected := []int{42, 42, 43, 44, 100, 101}
	if len(rows) != len(expected) {
		t.Fatalf("got %d numbered rows, want %d", len(rows), len(expected))
	}
	for index, row := range rows {
		if !row.numberKnown || row.number != expected[index] {
			t.Fatalf("row %d number=%d known=%t, want%d", index, row.number, row.numberKnown, expected[index])
		}
	}
	for _, raw := range []string{"-old\n+new", "@@ unknown\n-old\n+new", "@@ -bad +42 @@\n+new"} {
		for _, row := range inspectorDiffRows(raw) {
			if row.numberKnown {
				t.Fatalf("raw or malformed diff invented a line number: %+v", row)
			}
		}
	}
}

func TestInspectorDiffUsesFullWidthBackgroundsAndSingleHeader(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)
	model := referenceDashboard()
	selected, _ := model.selectedSession()
	cached := previewEntry{loaded: true, checked: time.Now(), edit: &agent.Edit{
		Filename: "sidebar.go", Diff: "@@ -42 +42 @@\n-old\n+new",
	}}
	const width = 70
	view := model.inspectorEdit(selected, cached, width, 8)
	lines := strings.Split(view, "\n")
	header := ansi.Strip(lines[0])
	for _, text := range []string{"LATEST EDIT", "sidebar.go", "+1", "−1"} {
		if !strings.Contains(header, text) {
			t.Fatalf("single header omitted %q: %s", text, header)
		}
	}
	for _, line := range lines {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "-old") || strings.Contains(plain, "+new") {
			if lipgloss.Width(line) != width || !strings.Contains(plain, "42 ") {
				t.Fatalf("diff row lost its number or full width: %q", plain)
			}
			background := "48;2;59;32;32"
			if strings.Contains(plain, "+new") {
				background = "48;2;25;58;38"
			}
			if !strings.Contains(line, background) {
				t.Fatalf("changed row has no soft background: %q", line)
			}
		}
	}
}

func TestInspectorActivityAnchorsWorkerSummaryInsteadOfThinking(t *testing.T) {
	model := referenceDashboard()
	selected, _ := model.selectedSession()
	model.sessions = append(model.sessions,
		agent.Session{ID: "worker-running", Provider: selected.Provider, ParentID: selected.ID, State: agent.StateRunning},
		agent.Session{ID: "worker-idle", Provider: selected.Provider, ParentID: selected.ID, State: agent.StateIdle},
		agent.Session{ID: "other-provider", Provider: "claude", ParentID: selected.ID, State: agent.StateRunning},
	)
	cached := previewEntry{loaded: true, checked: time.Now(), activity: &agent.Activity{
		Label: "Testing", Text: "Keyboard navigation passes.\nReviewing focus changes.", At: time.Now(),
	}}
	view := ansi.Strip(model.inspectorActivity(selected, cached, 60, 9))
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[2], "● ") || !strings.Contains(lines[len(lines)-1], "2 subagents · 1 running") ||
		strings.Contains(view, "is thinking") {
		t.Fatalf("activity lost its status dot or bottom worker summary:\n%s", view)
	}
}
