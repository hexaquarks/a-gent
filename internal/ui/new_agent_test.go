package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

type fakeLauncher struct {
	provider, directory string
	calls               int
	newWindow           bool
	err                 error
}

func (launcher *fakeLauncher) Launch(_ context.Context, provider, directory string, newWindow bool) error {
	launcher.provider, launcher.directory = provider, directory
	launcher.newWindow = newWindow
	launcher.calls++
	return launcher.err
}

func TestNewAgentDialogLaunchAndRetry(t *testing.T) {
	launcher := &fakeLauncher{err: errors.New("pane too small")}
	model := NewModel(nil, WithAgentLauncher(launcher))
	model.sessions = []agent.Session{{ID: "one", Provider: "codex", WorkingDirectory: "/projects/example"}}
	model.updateTableRows()
	model.width, model.height = 120, 35
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = next.(Model)
	if !strings.Contains(model.View(), "NEW AGENT") || model.newAgent.directory != "/projects/example" {
		t.Fatal("dialog did not open for selected project")
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	model = next.(Model)
	if model.previewExpanded {
		t.Fatal("dialog leaked preview key")
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = next.(Model)
	next, cmd := model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	_, duplicate := model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("started a duplicate launch")
	}
	next, _ = model.Update(cmd())
	model = next.(Model)
	if launcher.provider != "claude" || launcher.directory != "/projects/example" || launcher.calls != 1 || launcher.newWindow {
		t.Fatalf("wrong launch: %+v", launcher)
	}
	if model.newAgent.starting || !strings.Contains(model.View(), "pane too small") {
		t.Fatal("launch failure did not remain retryable")
	}
	launcher.err = nil
	next, cmd = model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	_, quit := model.Update(cmd())
	if quit == nil || quit() != tea.Quit() {
		t.Fatal("successful launch did not close dashboard")
	}
}

func TestNewAgentUsesFocusedProjectAndCancels(t *testing.T) {
	model := NewModel(nil)
	model.sessions = []agent.Session{{ID: "one", WorkingDirectory: "/projects/one"}, {ID: "two", WorkingDirectory: "/projects/two"}}
	model.sidebarFocus = true
	for index, item := range model.sidebarItems() {
		if item.project == "/projects/two" {
			model.sidebarCursor = index
		}
	}
	next, _ := model.openNewAgent(false)
	model = next.(Model)
	if model.newAgent.directory != "/projects/two" {
		t.Fatal("did not use focused project")
	}
	next, _ = model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.newAgent.err == nil {
		t.Fatal("missing tmux support was not explained")
	}
	next, _ = model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).newAgent != nil {
		t.Fatal("Esc did not cancel dialog")
	}
}

func TestCapitalNLaunchesNewWindow(t *testing.T) {
	launcher := &fakeLauncher{}
	model := NewModel(nil, WithAgentLauncher(launcher))
	model.width, model.height = 120, 35
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	model = next.(Model)
	if !strings.Contains(model.View(), "Open in a new tmux window") {
		t.Fatal("dialog did not show window destination")
	}
	next, cmd := model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	model.Update(cmd())
	if !launcher.newWindow || launcher.calls != 1 {
		t.Fatal("capital N did not launch in a new window")
	}
}

type blockingLauncher struct {
	started chan struct{}
}

func (launcher blockingLauncher) Launch(ctx context.Context, _, _ string, _ bool) error {
	close(launcher.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestQuitCancelsPendingAgentLaunch(t *testing.T) {
	launcher := blockingLauncher{started: make(chan struct{})}
	model := NewModel(nil, WithAgentLauncher(launcher))
	next, _ := model.openNewAgent(false)
	model = next.(Model)
	next, cmd := model.updateNewAgent(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-launcher.started:
	case <-time.After(time.Second):
		t.Fatal("launch did not start")
	}
	_, quit := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if quit == nil || quit() != tea.Quit() {
		t.Fatal("Ctrl+C did not quit")
	}
	select {
	case message := <-result:
		if !errors.Is(message.(agentLaunchMessage).err, context.Canceled) {
			t.Fatal("launch was not cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("launch continued after quit")
	}
}
