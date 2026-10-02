package codex

import (
	"testing"

	"a-gent/internal/agent"
)

func TestStateFromStatus(t *testing.T) {
	testCases := []struct {
		name     string
		status   threadStatus
		expected agent.State
	}{
		{name: "running", status: threadStatus{Type: "active"}, expected: agent.StateRunning},
		{name: "waiting for approval", status: threadStatus{Type: "active", ActiveFlags: []string{"waitingOnApproval"}}, expected: agent.StateWaiting},
		{name: "idle", status: threadStatus{Type: "idle"}, expected: agent.StateIdle},
		{name: "error", status: threadStatus{Type: "systemError"}, expected: agent.StateError},
		{name: "unknown", status: threadStatus{Type: "notLoaded"}, expected: agent.StateUnavailable},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := stateFromStatus(testCase.status)
			if actual != testCase.expected {
				t.Fatalf("stateFromStatus(%+v) = %q, want %q", testCase.status, actual, testCase.expected)
			}
		})
	}
}
