package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// Keep a representative six-session dashboard alongside the behavioral tests
// so changes to spacing, columns, and metadata can be reviewed as a whole.
func referenceDashboard() Model {
	now := time.Now()
	model := NewModel(nil, WithSessionNavigator(&fakeNavigator{}))
	model.sessions = []agent.Session{
		{
			ID:               "claude-session",
			Provider:         "claude",
			Name:             "Investigate terminal notifications",
			State:            agent.StateRunning,
			WorkingDirectory: "/projects/a-gent",
			LastActiveAt:     now.Add(-4500 * time.Millisecond),
		},
		{
			ID:               "codex-waiting",
			Provider:         "codex",
			Name:             "Investigate AGENTS.md override",
			State:            agent.StateWaiting,
			WorkingDirectory: "/projects/codex-audit",
			LastActiveAt:     now.Add(-18500 * time.Millisecond),
		},
		{
			ID:               "01a10404-3a82-7702-b4d9-3ae1bd0fb23e",
			Provider:         "codex",
			Name:             "Restore response navigation",
			State:            agent.StateIdle,
			WorkingDirectory: "/projects/swiftyprompt",
			LastActiveAt:     now.Add(-65 * time.Second),
		},
		{
			ID:               "codex-cursor",
			Provider:         "codex",
			Name:             "Align selected agent cursor",
			State:            agent.StateIdle,
			WorkingDirectory: "/projects/a-gent",
			LastActiveAt:     now.Add(-125 * time.Second),
		},
		{
			ID:               "codex-discovery",
			Provider:         "codex",
			Name:             "Add Claude session discovery",
			State:            agent.StateIdle,
			WorkingDirectory: "/projects/mihailanghelici",
			LastActiveAt:     now.Add(-485 * time.Second),
		},
		{
			ID:               "codex-untitled",
			Provider:         "codex",
			Name:             "Untitled session",
			State:            agent.StateIdle,
			WorkingDirectory: "/projects/swiftyprompt",
			LastActiveAt:     now.Add(-785 * time.Second),
		},
	}
	model.unreadSessions = map[sessionIdentity]bool{
		{provider: "codex", id: "codex-waiting"}:      true,
		{provider: "codex", id: model.sessions[2].ID}: true,
	}
	model.pinnedProjects = map[string]bool{"/projects/mihailanghelici": true}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: PopupWidth, Height: PopupHeight})
	model = updated.(Model)
	model.table.SetCursor(2)
	model.previewCache = map[sessionIdentity]previewEntry{
		{provider: "codex", id: model.sessions[2].ID}: {
			loaded: true, checked: now,
			edit: &agent.Edit{Filename: "internal/navigation.go", CompletedAt: now.Add(-65 * time.Second), Diff: "@@ openSession\n if session != nil {\n-    navigator.Open(cwd)\n+    navigator.Navigate(\n+        ctx, session,\n+    )\n }"},
		},
	}
	return model
}

func TestDashboardReferenceLayout(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)
	model := referenceDashboard()
	view := model.View()
	plain := ansi.Strip(view)
	if lipgloss.Width(view) != PopupWidth || lipgloss.Height(view) != PopupHeight {
		t.Fatalf("reference dashboard size = %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	for _, expected := range []string{
		"1 running", "1 needs input", "2 unseen", "Updates", "SESSIONS (6 of 6)",
		"Needs input", "SELECTED SESSION", "LAST EDIT", "internal/navigation.go",
		"@@ openSession", "+3 −1", "v expand", "hold order", model.sessions[2].ID,
	} {
		if !strings.Contains(plain, expected) {
			t.Errorf("reference dashboard missing %q:\n%s", expected, plain)
		}
	}
	headingRow := -1
	for index, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "SELECTED SESSION") && !strings.Contains(line, "LAST EDIT") {
			t.Error("selected-session and preview headings must share the same text row")
		}
		if lipgloss.Width(line) != PopupWidth {
			t.Errorf("row %d is %d cells wide", index, lipgloss.Width(line))
		}
		if strings.Contains(line, "VIEWS") {
			headingRow = index
			if !strings.Contains(line, "SESSIONS (6 of 6)") {
				t.Error("sidebar and session headings do not align")
			}
		}
	}
	if headingRow < 0 {
		t.Fatal("missing sidebar heading")
	}
	writeReferenceCapture(t, view, plain)
	expected, err := os.ReadFile("testdata/dashboard_reference.txt")
	if err != nil {
		t.Fatal(err)
	}
	if plain+"\n" != string(expected) {
		t.Fatalf("dashboard layout changed; review the reference capture:\n%s", plain)
	}
}

func writeReferenceCapture(t *testing.T, view, plain string) {
	t.Helper()
	destination := os.Getenv("A_GENT_TEST_ARTIFACTS")
	if destination == "" {
		return
	}
	directory := filepath.Join(destination, "reference")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	var frame strings.Builder
	frame.WriteString("\x1b[2J")
	for index, line := range strings.Split(view, "\n") {
		fmt.Fprintf(&frame, "\x1b[%d;1H%s", index+1, line)
	}
	frame.WriteString("\x1b[?25l")
	var cast strings.Builder
	encoder := json.NewEncoder(&cast)
	for _, record := range []any{
		map[string]any{"version": 2, "width": PopupWidth, "height": PopupHeight, "theme": map[string]string{"fg": colorMainText, "bg": colorBackground, "palette": "#000000:#cd0000:#00cd00:#cdcd00:#0000ee:#cd00cd:#00cdcd:#e5e5e5:#7f7f7f:#ff0000:#00ff00:#ffff00:#5c5cff:#ff00ff:#00ffff:#ffffff"}},
		[]any{0, "o", frame.String()}, []any{0.2, "o", ""},
	} {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	for extension, content := range map[string]string{"txt": plain + "\n", "ansi": view, "cast": cast.String()} {
		if err := os.WriteFile(filepath.Join(directory, "dashboard."+extension), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompactFooterKeepsAllActionsAndFrameVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {64, 20}, {40, 16}, {24, 10}} {
		model := referenceDashboard()
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model = updated.(Model)
		view := ansi.Strip(model.View())
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] || !strings.HasPrefix(lines[len(lines)-1], "╰") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
			t.Fatalf("%v: dashboard frame was clipped:\n%s", size, view)
		}
		footer := lines[len(lines)-2]
		for _, key := range []string{"tab", "j/k", "enter", "s", "f", "v", "q"} {
			if key == "enter" && size[0] < 40 {
				key = "↵"
			}
			if !strings.Contains(footer, key) {
				t.Errorf("%v: footer lost %s: %q", size, key, footer)
			}
		}
	}
}
