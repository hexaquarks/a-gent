package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
)

type blockingNavigator struct {
	started chan struct{}
}

func (navigator blockingNavigator) Navigate(ctx context.Context, _ agent.Session) error {
	close(navigator.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestRepeatedEnterWaitsForNavigationToFinish(t *testing.T) {
	navigator := &fakeNavigator{err: errors.New("temporary failure")}
	model := NewModel(nil, WithSessionNavigator(navigator))
	model.sessions = []agent.Session{{ID: "selected"}}

	updated, first := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if first == nil {
		t.Fatal("first Enter did not start navigation")
	}
	_, second := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if second != nil {
		t.Fatal("second Enter started a competing navigation operation")
	}

	updated, _ = model.Update(first())
	model = updated.(Model)
	_, retry := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if retry == nil {
		t.Fatal("failed navigation prevented a retry")
	}
	retry()
}

func TestQuitCancelsPendingNavigation(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			navigator := blockingNavigator{started: make(chan struct{})}
			model := NewModel(nil, WithSessionNavigator(navigator), WithApplicationContext(ctx))
			model.sessions = []agent.Session{{ID: "selected"}}
			updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(Model)

			result := make(chan tea.Msg, 1)
			go func() { result <- command() }()
			select {
			case <-navigator.started:
			case <-time.After(time.Second):
				t.Fatal("navigation did not start")
			}
			quitKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
			if key == "ctrl+c" {
				quitKey = tea.KeyMsg{Type: tea.KeyCtrlC}
			}
			_, quit := model.Update(quitKey)
			if quit == nil {
				t.Fatal("quit key did not close dashboard")
			}
			select {
			case message := <-result:
				if !errors.Is(message.(sessionNavigationMessage).err, context.Canceled) {
					t.Fatal("navigation was not cancelled on quit")
				}
			case <-time.After(time.Second):
				t.Fatal("navigation kept running after quit")
			}
		})
	}
}

func TestApplicationCancellationStopsNavigation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	navigator := blockingNavigator{started: make(chan struct{})}
	model := NewModel(nil, WithSessionNavigator(navigator), WithApplicationContext(ctx))
	model.sessions = []agent.Session{{ID: "selected"}}
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	result := make(chan tea.Msg, 1)
	go func() { result <- command() }()
	select {
	case <-navigator.started:
	case <-time.After(time.Second):
		t.Fatal("navigation did not start")
	}
	cancel()
	select {
	case message := <-result:
		if !errors.Is(message.(sessionNavigationMessage).err, context.Canceled) {
			t.Fatal("navigation did not receive application cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("navigation kept running after application shutdown")
	}
}

func TestQuitBeforeCommandRunsDoesNotStartNavigation(t *testing.T) {
	navigator := &fakeNavigator{}
	model := NewModel(nil, WithSessionNavigator(navigator))
	model.sessions = []agent.Session{{ID: "selected"}}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	message := command().(sessionNavigationMessage)
	if !errors.Is(message.err, context.Canceled) || navigator.session.ID != "" {
		t.Fatal("queued command started navigation after quit")
	}
}
