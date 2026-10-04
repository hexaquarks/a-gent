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

func TestThreadVisibility(t *testing.T) {
	testCases := []struct {
		name    string
		payload string
		visible bool
	}{
		{name: "empty idle GUI draft", payload: `{"source":"vscode","name":null,"preview":"","status":{"type":"idle"}}`, visible: true},
		{name: "empty idle CLI draft", payload: `{"source":"cli","name":null,"preview":"","status":{"type":"idle"}}`, visible: true},
		{name: "whitespace only draft", payload: `{"name":"  ","preview":"\n","status":{"type":"idle"}}`, visible: true},
		{name: "named idle conversation", payload: `{"name":"Fix dashboard","status":{"type":"idle"}}`, visible: true},
		{name: "untitled conversation with content", payload: `{"preview":"Help me fix this","status":{"type":"idle"}}`, visible: true},
		{name: "empty running conversation", payload: `{"status":{"type":"active"}}`, visible: true},
		{name: "empty error", payload: `{"status":{"type":"systemError"}}`, visible: true},
		{name: "unknown status", payload: `{"status":{"type":"futureStatus"}}`, visible: true},
		{name: "unloaded during polling", payload: `{"name":"Closed conversation","status":{"type":"notLoaded"}}`},
		{name: "running worker", payload: `{"parentThreadId":"main","status":{"type":"active"}}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var thread thread
			if err := json.Unmarshal([]byte(testCase.payload), &thread); err != nil {
				t.Fatal(err)
			}
			if got := thread.isVisibleSession(); got != testCase.visible {
				t.Fatalf("visible = %v, want %v", got, testCase.visible)
			}
		})
	}
}

func TestThreadSessionActivityTimestamp(t *testing.T) {
	testCases := []struct {
		name    string
		payload string
		seconds int64
	}{
		{name: "Unix seconds", payload: `{"name":"Conversation","updatedAt":1791028800}`, seconds: 1791028800},
		{name: "untitled conversation", payload: `{"preview":"User message","updatedAt":1791028800}`, seconds: 1791028800},
		{name: "synthetic empty thread timestamp", payload: `{"updatedAt":1791028800}`},
		{name: "missing", payload: `{}`},
		{name: "null", payload: `{"updatedAt":null}`},
		{name: "zero", payload: `{"updatedAt":0}`},
		{name: "negative", payload: `{"updatedAt":-1}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var loadedThread thread
			if err := json.Unmarshal([]byte(testCase.payload), &loadedThread); err != nil {
				t.Fatal(err)
			}

			session := loadedThread.session()
			if testCase.seconds == 0 {
				if !session.LastActiveAt.IsZero() {
					t.Fatalf("unknown timestamp = %v, want zero", session.LastActiveAt)
				}
				return
			}
			if actual := session.LastActiveAt.Unix(); actual != testCase.seconds {
				t.Fatalf("activity timestamp = %d, want %d Unix seconds", actual, testCase.seconds)
			}
		})
	}
}
