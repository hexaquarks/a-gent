package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

func TestFormatLastActiveAt(t *testing.T) {
	now := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	testCases := []struct {
		name         string
		lastActiveAt time.Time
		expected     string
	}{
		{name: "unknown", expected: "—"},
		{name: "just updated", lastActiveAt: now, expected: "0s ago"},
		{name: "future clock", lastActiveAt: now.Add(time.Minute), expected: "0s ago"},
		{name: "seconds", lastActiveAt: now.Add(-2500 * time.Millisecond), expected: "2s ago"},
		{name: "under a minute", lastActiveAt: now.Add(-59999 * time.Millisecond), expected: "59s ago"},
		{name: "one minute", lastActiveAt: now.Add(-time.Minute), expected: "1m ago"},
		{name: "minutes", lastActiveAt: now.Add(-5*time.Minute - 30*time.Second), expected: "5m ago"},
		{name: "under an hour", lastActiveAt: now.Add(-time.Hour + time.Millisecond), expected: "59m ago"},
		{name: "one hour", lastActiveAt: now.Add(-time.Hour), expected: "1h ago"},
		{name: "hours", lastActiveAt: now.Add(-3*time.Hour - 30*time.Minute), expected: "3h ago"},
		{name: "over a day", lastActiveAt: now.Add(-49 * time.Hour), expected: "49h ago"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := formatLastActiveAt(testCase.lastActiveAt, now)
			if actual != testCase.expected {
				t.Fatalf("formatted activity = %q, want %q", actual, testCase.expected)
			}
		})
	}
}

func TestLastActiveColumnAcrossTableWidths(t *testing.T) {
	for _, width := range []int{34, 48, 59, 60, 70, 89, 90, 120} {
		model := NewModel(nil)
		model.table.SetWidth(width)
		model.table.SetColumns(tableColumns(width))
		model.sessions = []agent.Session{{
			Provider:     "codex",
			State:        agent.StateIdle,
			LastActiveAt: time.Now().Add(-125 * time.Minute),
		}}
		model.updateTableRows()

		columns := model.table.Columns()
		if columns[len(columns)-1].Title != "Last active at" {
			t.Fatalf("width %d: activity column is not last", width)
		}
		if actual := model.table.Rows()[0][len(columns)-1]; actual != "2h ago" {
			t.Fatalf("width %d: activity cell = %q, want 2h ago", width, actual)
		}

		header := strings.Split(model.sessionTableView(), "\n")[0]
		if !strings.Contains(header, "Last active at") {
			t.Fatalf("width %d: activity header is truncated: %q", width, header)
		}
		row := model.sessionRowView(0, columns)
		if !strings.Contains(row, "2h ago") || lipgloss.Width(row) != width-panelStyle.GetHorizontalFrameSize() {
			t.Fatalf("width %d: unexpected activity row (%d cells): %q", width, lipgloss.Width(row), row)
		}
	}
}

func TestActivityAgeUsesCurrentSessionTimestamp(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{LastActiveAt: time.Now().Add(-125 * time.Minute)}}
	model.updateTableRows()

	// Rendering reads the timestamp rather than a previously formatted table cell.
	model.sessions[0].LastActiveAt = time.Now().Add(-185 * time.Minute)
	if row := model.sessionRowView(0, model.table.Columns()); !strings.Contains(row, "3h ago") {
		t.Fatalf("activity age is stale: %q", row)
	}
}
