package ui

import (
	"errors"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestBoardActivitySummaryUsesFreshSelectedPublicOutput(t *testing.T) {
	now := time.Now()
	session := agent.Session{Provider: "codex", ID: "one", State: agent.StateRunning}
	identity := sessionIdentity{provider: session.Provider, id: session.ID}

	tests := []struct {
		name   string
		change func(*Model, *previewEntry)
		want   string
	}{
		{name: "public output", want: "Assistant · Checking navigation"},
		{
			name: "first useful line and terminal controls",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity.Label = "\x1b[31mTool\x1b[0m\nresult"
				cached.activity.Text = "\n \t\n\x1b[2JChecked\x07 navigation\nsecond line"
			},
			want: "Tool result · Checked  navigation",
		},
		{
			name: "missing label",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity.Label = ""
			},
			want: "Checking navigation",
		},
		{
			name: "missing text",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity.Text = "\n\t"
			},
			want: "Assistant",
		},
		{
			name: "empty activity",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity.Label = ""
				cached.activity.Text = ""
			},
			want: "Working…",
		},
		{
			name: "old output from fresh read",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity.At = now.Add(-boardActivityFreshness - time.Second)
			},
			want: "Last output · Checking navigation",
		},
		{
			name: "future output timestamp",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity.At = now.Add(time.Second)
			},
			want: "Last output · Checking navigation",
		},
		{
			name: "freshness boundary",
			change: func(_ *Model, cached *previewEntry) {
				cached.checked = now.Add(-boardActivityFreshness)
			},
			want: "Assistant · Checking navigation",
		},
		{
			name: "expired cache",
			change: func(_ *Model, cached *previewEntry) {
				cached.checked = now.Add(-boardActivityFreshness - time.Nanosecond)
			},
			want: "Working…",
		},
		{
			name: "failed refresh",
			change: func(_ *Model, cached *previewEntry) {
				cached.err = errors.New("offline")
			},
			want: "Working…",
		},
		{
			name: "unloaded cache",
			change: func(_ *Model, cached *previewEntry) {
				cached.loaded = false
			},
			want: "Working…",
		},
		{
			name: "other selected session",
			change: func(model *Model, _ *previewEntry) {
				model.previewKey.id = "two"
			},
			want: "Working…",
		},
		{
			name: "same ID from another provider",
			change: func(model *Model, _ *previewEntry) {
				model.previewKey.provider = "claude"
			},
			want: "Working…",
		},
		{
			name: "completed edit without activity",
			change: func(_ *Model, cached *previewEntry) {
				cached.activity = nil
				cached.edit = &agent.Edit{Filename: "done.go"}
			},
			want: "Working…",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := Model{previewKey: identity}
			cached := previewEntry{
				loaded: true, checked: now,
				activity: &agent.Activity{Label: "Assistant", Text: "Checking navigation", At: now},
			}
			if test.change != nil {
				test.change(&model, &cached)
			}
			model.previewCache = map[sessionIdentity]previewEntry{identity: cached}

			if got := model.boardActivitySummary(session, now); got != test.want {
				t.Fatalf("summary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBoardActivitySummaryPrioritizesLiveState(t *testing.T) {
	now := time.Now()
	identity := sessionIdentity{provider: "codex", id: "one"}
	model := Model{
		previewKey: identity,
		previewCache: map[sessionIdentity]previewEntry{identity: {
			loaded: true, checked: now,
			activity: &agent.Activity{Label: "Tool requested", Text: "running command", At: now},
		}},
	}
	tests := []struct {
		state agent.State
		stale bool
		want  string
	}{
		{state: agent.StateWaiting, want: "Waiting for input"},
		{state: agent.StateIdle, want: "No current activity"},
		{state: agent.StateError, want: "Session error"},
		{state: agent.StateUnavailable, want: "Status unavailable"},
		{state: "", want: "Status unavailable"},
		{state: "unknown", want: "Status unavailable"},
		{state: agent.StateRunning, stale: true, want: "Status unavailable"},
		{state: agent.StateWaiting, stale: true, want: "Status unavailable"},
		{state: agent.StateError, stale: true, want: "Status unavailable"},
	}

	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			session := agent.Session{
				Provider: identity.provider, ID: identity.id, State: test.state, Stale: test.stale,
				Preview: "Conversation title is not current activity",
			}
			if got := model.boardActivitySummary(session, now); got != test.want {
				t.Fatalf("summary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBoardActivitySummaryShowsObservedCompletion(t *testing.T) {
	now := time.Now()
	session := agent.Session{Provider: "codex", ID: "one", State: agent.StateIdle}
	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	model := Model{finishedSessions: map[sessionIdentity]time.Time{identity: now}}

	if got := model.boardActivitySummary(session, now); got != "Finished this visit" {
		t.Fatalf("summary = %q, want observed completion", got)
	}
}
