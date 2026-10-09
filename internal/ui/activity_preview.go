package ui

import (
	"fmt"
	"strings"
	"time"

	"a-gent/internal/agent"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (model Model) showActivity(session agent.Session, cached previewEntry) bool {
	return cached.activity != nil && (session.State == agent.StateRunning || session.State == agent.StateWaiting || cached.edit == nil)
}

func (model Model) activityView(session agent.Session, cached previewEntry, width, height int, expanded bool) string {
	title := "LIVE ACTIVITY · v"
	if expanded {
		title = "LIVE ACTIVITY · Esc/q: return · j/k: scroll"
	}
	status := "Working · awaiting output"
	if time.Since(cached.activity.At) < 5*time.Second {
		status = "Working · live records"
	}
	if session.State == agent.StateWaiting {
		status = "Waiting for input"
	}
	if session.State == agent.StateIdle {
		status = "Last output · idle"
	}
	if cached.err != nil || session.Stale || time.Since(cached.checked) > 5*time.Second {
		status = "Stale · last recorded output"
	}
	if !cached.loaded {
		status = "Loading activity…"
	}
	lines := []string{accentStyle.Render(ansi.Truncate(title, width, "…")), mutedStyle.Render(ansi.Truncate(status, width, "…"))}
	activity := cached.activity
	lines = append(lines, mutedStyle.Render(ansi.Truncate(activity.Label+" · "+formatLastActiveAt(activity.At, time.Now()), width, "…")))
	textLines := strings.Split(activity.Text, "\n")
	for i, line := range textLines {
		textLines[i] = safeDisplayText(strings.ReplaceAll(line, "\t", "  "))
	}
	wrapped := strings.Split(ansi.Wrap(strings.Join(textLines, "\n"), width, ""), "\n")
	capacity := max(0, height-len(lines)-1)
	start := max(0, len(wrapped)-capacity)
	if expanded {
		start = min(model.previewScroll, max(0, len(wrapped)-capacity))
	}
	end := min(len(wrapped), start+capacity)
	for _, line := range wrapped[start:end] {
		lines = append(lines, mainTextStyle.Render(line))
	}
	if start > 0 || end < len(wrapped) {
		label := "… truncated · v: expand"
		if expanded {
			label = fmt.Sprintf("… lines %d–%d/%d", start+1, end, len(wrapped))
		}
		lines = append(lines, mutedStyle.Render(ansi.Truncate(label, width, "…")))
	}
	if len(lines) < height && !cached.checked.IsZero() {
		lines = append(lines, mutedStyle.Render(ansi.Truncate("Checked "+cached.checked.Format("15:04:05"), width, "…")))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}
