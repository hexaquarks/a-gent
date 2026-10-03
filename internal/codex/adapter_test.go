package codex

import (
	"encoding/json"
	"testing"

	"a-gent/internal/agent"
)

func TestThreadIsSubagent(t *testing.T) {
	testCases := []struct {
		name     string
		payload  string
		expected bool
	}{
		{name: "idle main conversation", payload: `{"source":"vscode","parentThreadId":null,"status":{"type":"idle"}}`},
		{name: "CLI conversation", payload: `{"source":"cli"}`},
		{name: "app server conversation", payload: `{"source":"appServer"}`},
		{name: "unknown source", payload: `{"source":"futureProvider"}`},
		{name: "missing metadata", payload: `{}`},
		{name: "null metadata", payload: `{"source":null,"parentThreadId":null}`},
		{name: "retained idle worker", payload: `{"source":{"subAgent":{"thread_spawn":{"parent_thread_id":"main","depth":1,"agent_path":"/root/session_limit"}}},"parentThreadId":"main","status":{"type":"idle"}}`, expected: true},
		{name: "running worker without parent field", payload: `{"source":{"subAgent":{"thread_spawn":{"parent_thread_id":"main"}}},"status":{"type":"active"}}`, expected: true},
		{name: "review worker", payload: `{"source":{"subAgent":"review"}}`, expected: true},
		{name: "compact worker", payload: `{"source":{"subAgent":"compact"}}`, expected: true},
		{name: "parent metadata without source", payload: `{"parentThreadId":"main"}`, expected: true},
		{name: "null worker tag", payload: `{"source":{"subAgent":null}}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var loadedThread thread
			if err := json.Unmarshal([]byte(testCase.payload), &loadedThread); err != nil {
				t.Fatal(err)
			}

			if actual := loadedThread.isSubagent(); actual != testCase.expected {
				t.Fatalf("isSubagent() = %v, want %v", actual, testCase.expected)
			}
		})
	}
}

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
