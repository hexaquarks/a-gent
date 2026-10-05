package ui

import (
	"errors"
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"
)

func TestProviderFailureDoesNotBlockHealthyUpdates(t *testing.T) {
	model := NewModel(nil)
	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "same", State: agent.StateRunning}}})
	model.applyProviderUpdate(polling.Update{Provider: "claude", Sessions: []agent.Session{{ID: "same", State: agent.StateIdle}}})
	model.applyProviderUpdate(polling.Update{Provider: "codex", Err: errors.New("offline")})
	model.applyProviderUpdate(polling.Update{Provider: "claude", Sessions: []agent.Session{{ID: "same", Name: "Updated", State: agent.StateRunning}}})
	if len(model.sessions) != 2 || model.lastError == nil {
		t.Fatalf("sessions = %+v, error = %v", model.sessions, model.lastError)
	}
	for _, session := range model.sessions {
		if session.Provider == "codex" {
			if !session.Stale || sessionState(session) != agent.StateUnavailable {
				t.Fatal("failed provider still appears live")
			}
			if matchesView(session, activeView) || !matchesView(session, attentionView) {
				t.Fatal("stale session is in the wrong view")
			}
			value, _ := sessionColumnValue(session, "Last active")
			if value == "Now" {
				t.Fatal("stale running session shows current activity")
			}
		} else if session.Name != "Updated" || session.Stale {
			t.Fatal("healthy provider did not update")
		}
	}
	if summary := summarizeSessions(model.sessions); summary.running != 1 || summary.errors != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(model.unreadSessions) != 0 {
		t.Fatal("provider failure invented a completion")
	}
	if !strings.Contains(model.renderSidebar(summarizeSessions(model.sessions)), "codex: unavailable") {
		t.Fatal("failed provider not identified")
	}

	model.applyProviderUpdate(polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "same", State: agent.StateIdle}}})
	if model.lastError != nil {
		t.Fatal("recovered provider retained an error")
	}
	if !model.unreadSessions[sessionIdentity{provider: "codex", id: "same"}] {
		t.Fatal("recovery lost the observed completion")
	}
	model.applyProviderUpdate(polling.Update{Provider: "codex"})
	if len(model.sessions) != 1 || model.sessions[0].Provider != "claude" {
		t.Fatal("empty result removed another provider")
	}
}

func TestSuccessfulRefreshDoesNotClearAnotherProvidersError(t *testing.T) {
	model := NewModel(nil)
	model.applyProviderUpdate(polling.Update{Provider: "codex", Err: errors.New("offline")})
	model.applyProviderUpdate(polling.Update{Provider: "claude", Err: errors.New("offline")})
	model.applyProviderUpdate(polling.Update{Provider: "claude"})
	if len(model.providerErrors) != 1 || model.providerErrors["codex"] == nil || model.lastError == nil {
		t.Fatal("success cleared another provider's failure")
	}
}

func TestUpdateStreamDrivesUIAndClosesCleanly(t *testing.T) {
	updates := make(chan polling.Update, 2)
	updates <- polling.Update{Provider: "codex", Sessions: []agent.Session{{ID: "one"}}}
	updates <- polling.Update{Provider: "claude", Sessions: []agent.Session{{ID: "two"}}}
	close(updates)
	model := NewModel(updates)
	command := model.Init()
	for range 2 {
		updated, next := model.Update(command())
		model = updated.(Model)
		command = next
	}
	if len(model.sessions) != 2 {
		t.Fatal("stream lost provider updates")
	}
	if message := command(); message != nil {
		t.Fatalf("closed stream returned %v", message)
	}
}
