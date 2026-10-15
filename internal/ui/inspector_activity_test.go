package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"github.com/charmbracelet/x/ansi"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestInspectorActivityExcerptKeepsLinkLabelsWithoutLongURLs(t *testing.T) {
	text := "Pushed and squash-merged [PR #26](https://github.com/hexaquarks/aiorganize/pull/26). " +
		"Local `main` is updated and clean."
	lines := inspectorActivityExcerpt(text, 50, 3)
	excerpt := strings.Join(lines, "\n")
	if !strings.Contains(excerpt, "PR #26") || !strings.Contains(excerpt, "main") ||
		strings.Contains(excerpt, "https://") || strings.Contains(excerpt, "`") {
		t.Fatalf("compact output retained noisy formatting or lost its meaning:\n%s", excerpt)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 50 {
			t.Fatalf("excerpt exceeded its column: %q", line)
		}
	}
}

func TestInspectorActivityExcerptMarksClippedOutput(t *testing.T) {
	lines := inspectorActivityExcerpt(strings.Repeat("Some more output. ", 20), 35, 2)
	if len(lines) != 2 || !strings.HasSuffix(lines[1], "…") {
		t.Fatalf("long output should end after two rows with an ellipsis: %q", lines)
	}
	if got := inspectorActivityExcerpt("output", 35, 0); len(got) != 0 {
		t.Fatal("no space should produce no excerpt")
	}
}

func TestInspectorActivityKeepsFullOutputInExpandedView(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)
	model := referenceDashboard()
	selected, _ := model.selectedSession()
	key := sessionIdentity{provider: selected.Provider, id: selected.ID}
	cached := model.previewCache[key]
	cached.activity = &agent.Activity{
		Label: "Assistant", At: time.Now(),
		Text: "Pushed [PR #26](https://github.com/hexaquarks/aiorganize/pull/26). Local `main` is clean.",
	}
	model.previewCache[key] = cached
	compact := ansi.Strip(model.detailWithPreview())
	if strings.Contains(compact, "https://") || strings.Contains(compact, "idle · Assistant") {
		t.Fatalf("inline output retained its noisy link or status: %s", compact)
	}
	writeReferenceCapture(t, model.View(), ansi.Strip(model.View()))
	model.previewExpanded = true
	model.previewShowsEdit = false
	expanded := ansi.Strip(model.View())
	if !strings.Contains(expanded, "https://github.com/hexaquarks/aiorganize/pull/26") {
		t.Fatal("compact formatting changed the full provider output")
	}
}
