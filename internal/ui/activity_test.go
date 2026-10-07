package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

func TestRunningSessionActivityUsesProviderTimestamp(t *testing.T) {
	for _, width := range []int{minimumTableWidth, 70, 120} {
		model := NewModel(nil)
		model.table.SetWidth(width)
		model.table.SetColumns(tableColumns(width))
		session := agent.Session{ID: "running", Provider: "codex", State: agent.StateRunning}
		for _, timestamp := range []time.Time{
			{}, time.Now().Add(-5 * time.Minute), time.Now(), time.Now().Add(time.Minute),
		} {
			session.LastActiveAt = timestamp
			updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{session}})
			model = updated.(Model)
			columns := model.table.Columns()
			expected := strings.TrimSuffix(formatLastActiveAt(timestamp, time.Now()), " ago")
			if timestamp.IsZero() {
				expected = "Now"
			}
			if got := model.table.Rows()[0][len(columns)-1]; got != expected {
				t.Fatalf("width %d: running activity = %q, want %s", width, got, expected)
			}
			row := ansi.Strip(model.sessionRowView(model.filteredSessions()[0], model.table.Cursor() == 0, columns))
			if !strings.HasSuffix(strings.TrimSpace(row), expected) {
				t.Fatalf("width %d: rendered activity is not Now: %q", width, row)
			}
			if !model.sessions[0].LastActiveAt.Equal(timestamp) {
				t.Fatal("displaying Now changed the timestamp used for activity sorting")
			}
		}

		for _, state := range []agent.State{agent.StateWaiting, agent.StateIdle, agent.StateError, agent.StateUnavailable} {
			session.State = state
			session.LastActiveAt = time.Now().Add(-125 * time.Minute)
			updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{session}})
			model = updated.(Model)
			if row := model.sessionRowView(model.filteredSessions()[0], model.table.Cursor() == 0, model.table.Columns()); !strings.HasSuffix(strings.TrimSpace(ansi.Strip(row)), "2h") {
				t.Fatalf("width %d: %s session did not resume elapsed activity: %q", width, state, row)
			}
		}
		session.State = agent.StateIdle
		session.LastActiveAt = time.Time{}
		updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{session}})
		model = updated.(Model)
		if row := model.sessionRowView(model.filteredSessions()[0], model.table.Cursor() == 0, model.table.Columns()); !strings.HasSuffix(strings.TrimSpace(ansi.Strip(row)), "—") {
			t.Fatalf("width %d: unknown idle activity is not preserved: %q", width, row)
		}
	}
}

func TestLastActiveColumnAcrossTableWidths(t *testing.T) {
	for _, width := range []int{minimumTableWidth, 48, 59, 60, 70, 89, 90, 120} {
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
		if columns[len(columns)-1].Title != "Last active" {
			t.Fatalf("width %d: activity column is not last", width)
		}
		if actual := model.table.Rows()[0][len(columns)-1]; actual != "2h" {
			t.Fatalf("width %d: activity cell = %q, want 2h ago", width, actual)
		}

		header := strings.Split(model.sessionTableView(), "\n")[0]
		if !strings.Contains(header, "Last active") {
			t.Fatalf("width %d: activity header is truncated: %q", width, header)
		}
		row := model.sessionRowView(model.filteredSessions()[0], model.table.Cursor() == 0, columns)
		if !strings.Contains(row, "2h") || lipgloss.Width(row) != width-panelStyle.GetHorizontalFrameSize()-scrollbarGutterWidth {
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
	if row := model.sessionRowView(model.filteredSessions()[0], model.table.Cursor() == 0, model.table.Columns()); !strings.Contains(row, "3h") {
		t.Fatalf("activity age is stale: %q", row)
	}
}
