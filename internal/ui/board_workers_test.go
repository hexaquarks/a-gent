package ui

import (
	"strings"
	"testing"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestBoardWorkerSummaryIncludesNestedAttention(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previousProfile)

	model := NewModel(nil)
	parent := agent.Session{ID: "root", Provider: "codex", State: agent.StateRunning}
	model.sessions = []agent.Session{
		parent,
		{ID: "child", Provider: "codex", ParentID: parent.ID, State: agent.StateRunning},
		{ID: "nested", Provider: "codex", ParentID: "child", State: agent.StateWaiting},
		{ID: "stale", Provider: "codex", ParentID: "nested", State: agent.StateRunning, Stale: true},
		{ID: "error", Provider: "codex", ParentID: "child", State: agent.StateError},
		{ID: "idle", Provider: "codex", ParentID: parent.ID, State: agent.StateIdle},
		{ID: "other-provider", Provider: "claude", ParentID: parent.ID, State: agent.StateWaiting},
		{ID: "orphan", Provider: "codex", ParentID: "missing", State: agent.StateWaiting},
		// Malformed ancestry must not count the root twice or loop forever.
		{ID: parent.ID, Provider: "codex", ParentID: "nested", State: agent.StateWaiting},
	}
	view := model.boardCardView(parent, false, 44)
	if !strings.Contains(ansi.Strip(view), "5 subagents · 3 needs you") {
		t.Fatalf("nested attention disappeared from parent summary:\n%s", ansi.Strip(view))
	}
	attentionText := waitingStyle.Background(lipgloss.Color(colorSection)).Render("3 needs you")
	if !strings.Contains(view, attentionText) {
		t.Fatal("worker attention summary did not use amber")
	}
	if lipgloss.Height(view) != model.boardCardHeight(parent) {
		t.Fatal("nested workers changed card height without updating layout")
	}
}

func TestBoardWorkerSummaryUsesRunningCountWhenNoAttentionRemains(t *testing.T) {
	model := NewModel(nil)
	parent := agent.Session{ID: "root", Provider: "codex", State: agent.StateRunning}
	model.sessions = []agent.Session{
		parent,
		{ID: "child", Provider: "codex", ParentID: parent.ID, State: agent.StateRunning},
		{ID: "nested", Provider: "codex", ParentID: "child", State: agent.StateRunning},
	}
	view := ansi.Strip(model.boardCardView(parent, false, 44))
	if !strings.Contains(view, "2 subagents · 2 running") {
		t.Fatalf("nested running workers were omitted:\n%s", view)
	}

	model.sessions = model.sessions[:2]
	view = ansi.Strip(model.boardCardView(parent, false, 44))
	if !strings.Contains(view, "1 subagent · 1 running") {
		t.Fatalf("single worker label is not singular:\n%s", view)
	}
}
