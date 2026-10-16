package ui

import (
	"testing"
	"time"

	"a-gent/internal/agent"
)

func refreshFinishedTestModel(model *Model, now time.Time, sessions ...agent.Session) {
	model.updateUnreadSessionsAt(sessions, now)
	model.sessions = sessions
}

func TestFinishedHintRequiresObservedRunningToIdle(t *testing.T) {
	now := time.Now()
	for _, initial := range []agent.State{
		agent.StateIdle, agent.StateWaiting, agent.StateError, agent.StateUnavailable, agent.StateRunning,
	} {
		t.Run(string(initial), func(t *testing.T) {
			model := NewModel(nil)
			session := agent.Session{ID: "one", Provider: "codex", State: initial}
			refreshFinishedTestModel(&model, now, session)
			if model.finishedThisVisit(session, now) {
				t.Fatal("initial snapshot showed a finished hint")
			}

			session.State = agent.StateIdle
			refreshFinishedTestModel(&model, now, session)
			if got := model.finishedThisVisit(session, now); got != (initial == agent.StateRunning) {
				t.Fatalf("finished hint after %s → idle = %v", initial, got)
			}
		})
	}
}

func TestFinishedHintExpiresWithoutRefreshingItsLifetime(t *testing.T) {
	now := time.Now()
	model := NewModel(nil)
	session := agent.Session{ID: "one", Provider: "codex", State: agent.StateRunning}
	refreshFinishedTestModel(&model, now, session)
	session.State = agent.StateIdle
	refreshFinishedTestModel(&model, now, session)

	later := now.Add(finishedHintDuration / 2)
	refreshFinishedTestModel(&model, later, session)
	if !model.finishedThisVisit(session, later) {
		t.Fatal("idle refresh removed the finished hint too early")
	}

	expiresAt := now.Add(finishedHintDuration)
	if model.finishedThisVisit(session, expiresAt) {
		t.Fatal("finished hint remained visible after its lifetime")
	}
	refreshFinishedTestModel(&model, expiresAt, session)
	if len(model.finishedSessions) != 0 {
		t.Fatal("expired finish record remained after refresh")
	}
}

func TestFinishedHintPreservesRecordThroughFailedPoll(t *testing.T) {
	now := time.Now()
	model := NewModel(nil)
	session := agent.Session{ID: "one", Provider: "codex", State: agent.StateRunning}
	refreshFinishedTestModel(&model, now, session)

	stale := session
	stale.State = agent.StateIdle
	stale.Stale = true
	refreshFinishedTestModel(&model, now, stale)
	if len(model.finishedSessions) != 0 {
		t.Fatal("stale idle state created a finished hint")
	}

	refreshFinishedTestModel(&model, now, session)
	session.State = agent.StateIdle
	refreshFinishedTestModel(&model, now, session)
	refreshFinishedTestModel(&model, now, stale)
	if len(model.finishedSessions) != 1 || model.finishedThisVisit(stale, now) {
		t.Fatal("failed poll should preserve the record without showing a fresh completion")
	}

	refreshFinishedTestModel(&model, now, session)
	if !model.finishedThisVisit(session, now) {
		t.Fatal("successful idle poll did not restore the finished hint")
	}
}

func TestFinishedHintClearsWhenSessionChangesOrDisappears(t *testing.T) {
	now := time.Now()
	for _, state := range []agent.State{agent.StateRunning, agent.StateWaiting, agent.StateError} {
		t.Run(string(state), func(t *testing.T) {
			model := NewModel(nil)
			session := agent.Session{ID: "one", Provider: "codex", State: agent.StateRunning}
			refreshFinishedTestModel(&model, now, session)
			session.State = agent.StateIdle
			refreshFinishedTestModel(&model, now, session)
			session.State = state
			refreshFinishedTestModel(&model, now, session)
			if len(model.finishedSessions) != 0 {
				t.Fatalf("%s session kept its finished hint", state)
			}
		})
	}

	model := NewModel(nil)
	session := agent.Session{ID: "same", Provider: "codex", State: agent.StateRunning}
	other := agent.Session{ID: "same", Provider: "claude", State: agent.StateIdle}
	refreshFinishedTestModel(&model, now, session, other)
	session.State = agent.StateIdle
	refreshFinishedTestModel(&model, now, session, other)
	if model.finishedThisVisit(other, now) {
		t.Fatal("finished hint leaked to another provider with the same session ID")
	}
	refreshFinishedTestModel(&model, now, other)
	refreshFinishedTestModel(&model, now, session, other)
	if model.finishedThisVisit(session, now) {
		t.Fatal("removed session regained its old finished hint")
	}
}

func TestAcknowledgingSessionClearsFinishedHint(t *testing.T) {
	now := time.Now()
	model := NewModel(nil)
	session := agent.Session{ID: "one", Provider: "codex", State: agent.StateRunning}
	refreshFinishedTestModel(&model, now, session)
	session.State = agent.StateIdle
	refreshFinishedTestModel(&model, now, session)
	model.updateTableRows()
	model.markSelectedSessionRead()
	refreshFinishedTestModel(&model, now, session)
	if model.finishedThisVisit(session, now) || len(model.unreadSessions) != 0 {
		t.Fatal("acknowledged session retained its finished hint or unseen mark")
	}
}
