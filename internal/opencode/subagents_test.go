package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"a-gent/internal/agent"
)

func TestSubagentRuntimeOwnershipAndLifecycle(t *testing.T) {
	child := func(id, parent string) sessionRecord {
		record := fixtureRecord(id)
		record.ParentID = parent
		return record
	}
	// Children can precede their parents in the native response. Intermediate
	// ancestors of an active nested task may already have become idle.
	records := []sessionRecord{
		child("nested", "child"), child("child", "root"), fixtureRecord("root"),
		child("saved", "root"), child("orphan", "missing"),
		child("cycle-one", "cycle-two"), child("cycle-two", "cycle-one"),
		child("archived", "root"), child("elsewhere", "root"), child("waiting", "root"),
	}
	records[7].Time.Archived = 1
	records[8].Directory = "/other-project"
	statuses := map[string]statusRecord{
		"root": {Type: "busy"}, "nested": {Type: "retry"}, "orphan": {Type: "busy"},
		"cycle-one": {Type: "busy"}, "archived": {Type: "busy"}, "elsewhere": {Type: "busy"},
	}
	pending := []pendingRequest{{SessionID: "waiting"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session":
			json.NewEncoder(w).Encode(records)
		case "/session/status":
			json.NewEncoder(w).Encode(statuses)
		case "/permission":
			json.NewEncoder(w).Encode(pending)
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer server.Close()
	adapter := fixtureAdapter(server, instance{pid: 42, directory: "/project"})

	assertSessions := func(expected map[string]agent.State) map[string]agent.Session {
		t.Helper()
		sessions, err := adapter.Sessions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		states := make(map[string]agent.State)
		byID := make(map[string]agent.Session)
		for _, session := range sessions {
			states[session.TranscriptID] = session.State
			byID[session.TranscriptID] = session
		}
		if !reflect.DeepEqual(states, expected) {
			t.Fatalf("runtime sessions: %v; expected %v", states, expected)
		}
		return byID
	}
	found := assertSessions(map[string]agent.State{
		"root": agent.StateRunning, "child": agent.StateIdle, "nested": agent.StateRunning,
		"waiting": agent.StateWaiting,
	})
	if found["child"].ParentID != found["root"].ID || found["nested"].ParentID != found["child"].ID {
		t.Fatalf("parent IDs do not match dashboard identities: %+v", found)
	}

	clear(statuses)
	pending = []pendingRequest{}
	assertSessions(map[string]agent.State{
		"root": agent.StateIdle, "child": agent.StateIdle, "nested": agent.StateIdle,
		"waiting": agent.StateIdle,
	})
	// Archiving a parent removes its descendants even when the old child record
	// remains in shared history and the adapter observed it during execution.
	records[1].Time.Archived = 1
	assertSessions(map[string]agent.State{"root": agent.StateIdle, "waiting": agent.StateIdle})
	records = []sessionRecord{}
	assertSessions(map[string]agent.State{"": agent.StateIdle})
}
