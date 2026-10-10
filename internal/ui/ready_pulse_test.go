package ui

import (
	"errors"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"
	"github.com/charmbracelet/lipgloss"
)

func TestReadyPulseTransitions(t *testing.T) {
	for _, state := range []agent.State{agent.StateIdle, agent.StateWaiting, agent.StateError, agent.StateUnavailable} {
		t.Run(string(state), func(t *testing.T) {
			session := agent.Session{ID: "same", Provider: "codex", State: state}
			other := agent.Session{ID: "same", Provider: "claude", State: agent.StateIdle}
			model := refreshUnreadTestModel(NewModel(nil), session, other)
			if len(model.readyPulses) != 0 {
				t.Fatal("initial discovery animated")
			}
			session.State = agent.StateRunning
			model = refreshUnreadTestModel(model, session, other)
			failed, _ := model.Update(polling.Update{Provider: "codex", Err: errors.New("offline")})
			model = failed.(Model)
			if len(model.readyPulses) != 0 {
				t.Fatal("failed poll animated")
			}
			session.State = state
			model = refreshUnreadTestModel(model, session, other)
			identity := sessionIdentity{provider: "codex", id: "same"}
			started, animated := model.readyPulses[identity]
			want := state == agent.StateIdle || state == agent.StateWaiting
			if animated != want || len(model.readyPulses) > 1 {
				t.Fatalf("wrong transitions: %v", model.readyPulses)
			}
			if !want {
				return
			}
			model = refreshUnreadTestModel(model, session, other)
			if model.readyPulses[identity] != started {
				t.Fatal("unchanged poll restarted pulse")
			}
			model.sort = sessionSort{column: "Session", descending: true}
			model.updateTableRows()
			if model.readyPulses[identity] != started {
				t.Fatal("sorting lost pulse identity")
			}
			session.State = agent.StateRunning
			model = refreshUnreadTestModel(model, session, other)
			if len(model.readyPulses) != 0 {
				t.Fatal("resumed session retained pulse")
			}
			session.State = state
			model = refreshUnreadTestModel(model, session, other)
			if len(model.readyPulses) != 1 {
				t.Fatal("second completion did not animate")
			}
			model = refreshUnreadTestModel(model, other)
			if len(model.readyPulses) != 0 {
				t.Fatal("removed session retained pulse")
			}
		})
	}
}

func TestReadyPulseFadesAndStopsTimer(t *testing.T) {
	session := agent.Session{ID: "one", Provider: "codex", State: agent.StateRunning}
	model := refreshUnreadTestModel(NewModel(nil), session)
	session.State = agent.StateIdle
	model = refreshUnreadTestModel(model, session)
	identity := sessionIdentity{provider: "codex", id: "one"}
	started := model.readyPulses[identity]
	for _, base := range []lipgloss.Color{"", colorSelection} {
		model.readyPulseTime = started.Add(readyPulseDuration / 2)
		peak := model.readyPulseBackground(session, base)
		model.readyPulseTime = started.Add(readyPulseDuration * 9 / 10)
		fading := model.readyPulseBackground(session, base)
		if peak == base || fading == peak {
			t.Fatal("background did not animate")
		}
	}
	updated, command := model.update(readyPulseMessage(started.Add(readyPulseDuration / 2)))
	model = updated.(Model)
	if command == nil {
		t.Fatal("animation did not schedule its next frame")
	}
	updated, command = model.update(readyPulseMessage(started.Add(readyPulseDuration)))
	model = updated.(Model)
	if command != nil || model.readyPulseTicking || len(model.readyPulses) != 0 {
		t.Fatal("animation did not stop")
	}
	for _, base := range []lipgloss.Color{"", colorSelection} {
		if model.readyPulseBackground(session, base) != base {
			t.Fatal("normal background was not restored")
		}
	}
	if !model.unreadSessions[identity] {
		t.Fatal("animation cleared unread indicator")
	}
	model = refreshUnreadTestModel(model, session)
	if len(model.readyPulses) != 0 {
		t.Fatal("completed pulse restarted on next poll")
	}
}
