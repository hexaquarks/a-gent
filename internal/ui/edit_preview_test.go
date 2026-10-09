package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/codex"
	"a-gent/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type previewSource struct {
	calls   chan string
	release chan struct{}
}

func (source *previewSource) Provider() string                                  { return "codex" }
func (source *previewSource) Sessions(context.Context) ([]agent.Session, error) { return nil, nil }
func (source *previewSource) LatestPreview(_ context.Context, session agent.Session) (agent.Preview, error) {
	source.calls <- session.ID
	<-source.release // Deliberately ignores cancellation to test late results.
	return agent.Preview{Edit: &agent.Edit{Filename: session.ID + ".go", CompletedAt: time.Now(), Diff: "@@ -1 +1 @@\n-old\n+new"}}, nil
}

func TestPreviewSwitchRejectsDelayedResponse(t *testing.T) {
	source := &previewSource{calls: make(chan string, 2), release: make(chan struct{})}
	model := NewModel(nil, WithPreviewSources(source))
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "a", Name: "Alpha"}, {ID: "b", Name: "Beta"}}})
	model.previewCache = make(map[sessionIdentity]previewEntry)
	first := model.syncPreview()
	results := make(chan tea.Msg, 1)
	go func() { results <- first() }()
	oldID := <-source.calls
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	selected, _ := model.selectedSession()
	if selected.ID == oldID || model.previewKey.id != selected.ID {
		t.Fatal("selection did not immediately switch preview")
	}
	close(source.release)
	updated, _ = model.Update(<-results)
	model = updated.(Model)
	if len(model.previewCache) != 0 {
		t.Fatal("late result populated cache")
	}
	if command == nil {
		t.Fatal("new selection did not load")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if !strings.Contains(model.previewView(40, 9, false), selected.ID+".go") {
		t.Fatal("new preview missing")
	}
	if len(source.calls) != 1 {
		t.Fatal("loaded unexpected session histories")
	}
}

func TestPreviewRetainsIdleAndStaleEdit(t *testing.T) {
	source := &previewSource{}
	model := NewModel(nil, WithPreviewSources(source))
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "a", State: agent.StateRunning}}})
	key := sessionIdentity{provider: "codex", id: "a"}
	edit := &agent.Edit{Filename: "kept.go", Diff: "+hello", CompletedAt: time.Now()}
	model.previewKey = key
	model.previewCache = map[sessionIdentity]previewEntry{key: {edit: edit, loaded: true, checked: time.Now()}}
	updated, _ := model.Update(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "a", State: agent.StateIdle}}})
	model = updated.(Model)
	if !strings.Contains(model.previewView(40, 9, false), "kept.go") {
		t.Fatal("idle cleared edit")
	}
	updated, _ = model.Update(previewResult{key: key, revision: model.previewRevision, err: errors.New("offline")})
	model = updated.(Model)
	view := model.previewView(40, 9, false)
	if !strings.Contains(view, "kept.go") || !strings.Contains(view, "stale") {
		t.Fatal(view)
	}
	updated, _ = model.Update(previewResult{key: key, revision: model.previewRevision, edit: &agent.Edit{Filename: "new.go", Diff: "+new"}})
	model = updated.(Model)
	if !strings.Contains(model.previewView(40, 9, false), "new.go") {
		t.Fatal("new completion did not replace cached edit")
	}
}

func TestSuccessfulEmptyPreviewClearsReplacedTranscript(t *testing.T) {
	model := NewModel(nil, WithPreviewSources(codex.NewAdapter()))
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "a", State: agent.StateRunning}}})
	key := sessionIdentity{provider: "codex", id: "a"}
	model.previewKey = key
	model.previewCache = map[sessionIdentity]previewEntry{key: {
		loaded: true, checked: time.Now(),
		edit: &agent.Edit{Filename: "kept.go", Diff: "+kept"},
	}}
	if view := model.previewView(40, 9, false); !strings.Contains(view, "kept.go") {
		t.Fatal("waiting for activity hid the previous completed edit")
	}
	updated, _ := model.Update(previewResult{key: key})
	model = updated.(Model)
	if view := model.previewView(40, 9, false); strings.Contains(view, "kept.go") || !strings.Contains(view, "Waiting for output") {
		t.Fatalf("successful empty read retained obsolete content: %s", view)
	}
}

func TestPreviewStatesAndTerminalBounds(t *testing.T) {
	for _, size := range [][2]int{{120, 27}, {100, 23}, {80, 24}, {64, 20}, {40, 16}, {24, 10}} {
		model := NewModel(nil, WithPreviewSources(&previewSource{}))
		model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "a", Name: strings.Repeat("session", 30), WorkingDirectory: strings.Repeat("/directory", 30)}}})
		model.width = size[0]
		model.height = size[1]
		model.resizeTable()
		key := sessionIdentity{provider: "codex", id: "a"}
		model.previewCache = map[sessionIdentity]previewEntry{key: {loaded: true, checked: time.Now()}}
		if !strings.Contains(model.previewView(30, 7, false), "No edits yet") {
			t.Fatal("missing empty state")
		}
		cached := model.previewCache[key]
		cached.err = errors.New("offline")
		model.previewCache[key] = cached
		if !strings.Contains(model.previewView(30, 7, false), "Preview unavailable") {
			t.Fatal("missing unavailable state")
		}
		cached.err = nil
		cached.edit = &agent.Edit{Filename: "file.go", Diff: "@@ -1 +1 @@\n-context\n+" + strings.Repeat("界", 100) + "\n@@ -50 +50 @@\n+other"}
		model.previewCache[key] = cached
		if !strings.Contains(model.previewView(30, 7, false), "truncated") {
			t.Fatal("truncation hidden")
		}
		for _, expanded := range []bool{false, true} {
			model.previewExpanded = expanded
			view := model.View()
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%v expanded=%v: %dx%d", size, expanded, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model = updated.(Model)
		if model.previewExpanded {
			t.Fatal("escape did not return")
		}
		if size[0] >= 100 && size[1] >= 27 && !strings.Contains(ansi.Strip(model.View()), "file.go") {
			t.Fatal("preview not visible beside metadata")
		}
	}
}

func TestCompactHunkShowsChangesAfterContext(t *testing.T) {
	model := NewModel(nil)
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one"}}})
	key := sessionIdentity{provider: "codex", id: "one"}
	model.previewCache = map[sessionIdentity]previewEntry{key: {loaded: true, checked: time.Now(), edit: &agent.Edit{Filename: "file.go", Diff: "@@ -1,5 +1,5 @@\n context1\n context2\n context3\n-old\n+new\n after"}}}
	view := ansi.Strip(model.previewView(30, 7, false))
	if !strings.Contains(view, "-old") || !strings.Contains(view, "+new") || !strings.Contains(view, "truncated") {
		t.Fatal(view)
	}
}

func TestPreviewCacheSeparatesProvidersAndRejectsOldRevision(t *testing.T) {
	model := NewModel(nil)
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "same"}}})
	model.applyProviderUpdate(polling.Update{Provider: "claude", Sessions: []agent.Session{{ID: "same"}}})
	model.previewCache = map[sessionIdentity]previewEntry{
		{provider: "codex", id: "same"}:  {loaded: true, checked: time.Now(), edit: &agent.Edit{Filename: "codex.go", Diff: "+codex"}},
		{provider: "claude", id: "same"}: {loaded: true, checked: time.Now(), edit: &agent.Edit{Filename: "claude.go", Diff: "+claude"}},
	}
	model.table.SetCursor(0)
	model.syncPreview()
	selected, _ := model.selectedSession()
	if !strings.Contains(model.previewView(30, 7, false), selected.Provider+".go") {
		t.Fatal("wrong provider cache")
	}
	previous := model.previewKey
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	selected, _ = model.selectedSession()
	if model.previewKey == previous || !strings.Contains(model.previewView(30, 7, false), selected.Provider+".go") {
		t.Fatal("provider selection did not switch immediately")
	}
	updated, _ = model.Update(previewResult{key: model.previewKey, revision: model.previewRevision - 1, edit: &agent.Edit{Filename: "late.go"}})
	model = updated.(Model)
	if strings.Contains(model.previewView(30, 7, false), "late.go") {
		t.Fatal("old revision overwrote current preview")
	}
}

func TestProjectSearchOwnsPreviewShortcut(t *testing.T) {
	model := NewModel(nil)
	model = sendProjectKey(model, runeKey('/'))
	model = sendProjectKey(model, runeKey('v'))
	if model.previewExpanded || model.projectQuery != "v" {
		t.Fatal("preview shortcut intercepted search input")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.projectSearching {
		t.Fatal("escape did not leave project search")
	}
	model = sendProjectKey(model, runeKey('v'))
	if !model.previewExpanded {
		t.Fatal("preview shortcut stopped working outside search")
	}
}

func TestQuitClosesExpandedPreviewBeforeDashboard(t *testing.T) {
	model := NewModel(nil)
	model.width, model.height = 80, 24
	model.previewExpanded = true

	updated, command := model.Update(runeKey('q'))
	model = updated.(Model)
	if command != nil || model.previewExpanded || !strings.Contains(model.View(), "SELECTED SESSION") {
		t.Fatal("q did not return from the preview to the dashboard")
	}

	_, command = model.Update(runeKey('q'))
	if command == nil || command() != tea.Quit() {
		t.Fatal("q did not quit from the dashboard")
	}
}

func TestPreviewTimerReadsNewCompletedEditWithoutSessionUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	model := NewModel(nil, WithPreviewSources(codex.NewAdapter()))
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one", State: agent.StateIdle, TranscriptPath: path}}})
	model.previewCache = make(map[sessionIdentity]previewEntry)
	command := model.syncPreview()
	updated, _ := model.Update(command())
	model = updated.(Model)
	if !strings.Contains(model.previewView(60, 8, true), "No edits yet") {
		t.Fatal("missing empty preview state")
	}

	for _, filename := range []string{"first.go", "second.go"} {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = file.WriteString(`{"type":"event_msg","timestamp":"2026-10-05T12:00:00Z","payload":{"type":"item_completed","thread_id":"one","item":{"type":"FileChange","status":"completed","changes":{"` + filename + `":{"type":"update","unified_diff":"@@ -1 +1 @@\n-old\n+new"}}}}}` + "\n")
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		cached := model.previewCache[model.previewKey]
		cached.checked = time.Now().Add(-3 * time.Second)
		model.previewCache[model.previewKey] = cached
		updated, command = model.Update(previewTick{})
		model = updated.(Model)
		// The timer produces a next tick and a selected-session read. Execute
		// the read immediately without waiting for another provider poll.
		batch, ok := command().(tea.BatchMsg)
		if !ok || len(batch) != 2 {
			t.Fatal("timer did not schedule the next refresh and edit read")
		}
		updated, _ = model.Update(batch[1]())
		model = updated.(Model)
		view := model.previewView(80, 10, true)
		if !strings.Contains(view, filename) || !strings.Contains(view, "checked ") {
			t.Fatalf("new completion not visible: %s", view)
		}
	}
}
