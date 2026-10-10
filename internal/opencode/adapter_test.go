package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func fixtureRecord(id string) sessionRecord {
	record := sessionRecord{ID: id, Title: "OpenCode " + id, Directory: "/project"}
	record.Time.Created = 1000
	record.Time.Updated = 2000
	return record
}

func fixtureAdapter(server *httptest.Server, instances ...instance) Adapter {
	adapter := NewAdapter()
	for i := range instances {
		instances[i].endpoint = server.URL
	}
	adapter.discover = func(context.Context) ([]instance, error) { return instances, nil }
	return adapter
}

func TestSessionsNativeStatusAndIdleRetention(t *testing.T) {
	records := []sessionRecord{fixtureRecord("ses_busy"), fixtureRecord("ses_permission"), fixtureRecord("ses_question"), fixtureRecord("ses_history")}
	statuses := map[string]statusRecord{"ses_busy": {Type: "busy"}, "ses_permission": {Type: "busy"}, "ses_question": {Type: "retry"}}
	pending := map[string][]pendingRequest{"/permission": {{SessionID: "ses_permission"}}, "/question": {{SessionID: "ses_question"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("directory") != "/project" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/session":
			json.NewEncoder(w).Encode(records)
		case "/session/status":
			json.NewEncoder(w).Encode(statuses)
		case "/permission", "/question":
			requests := pending[r.URL.Path]
			if requests == nil {
				requests = []pendingRequest{}
			}
			json.NewEncoder(w).Encode(requests)
		default:
			t.Errorf("unexpected read: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter := fixtureAdapter(server, instance{pid: 42, directory: "/project", started: time.Unix(100, 0)})
	sessions, err := adapter.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	states := make(map[string]agent.State)
	for _, session := range sessions {
		states[session.TranscriptID] = session.State
		if session.Provider != "opencode" || *session.ProcessID != 42 || session.Name == "" || session.LastActiveAt.UnixMilli() != 2000 {
			t.Fatalf("bad metadata: %+v", session)
		}
	}
	expected := map[string]agent.State{"ses_busy": agent.StateRunning, "ses_permission": agent.StateWaiting, "ses_question": agent.StateWaiting}
	if !reflect.DeepEqual(states, expected) {
		t.Fatalf("states: %v", states)
	}
	// Idle entries disappear from OpenCode's native status map, not its history.
	clear(statuses)
	clear(pending)
	idle, err := adapter.Sessions(context.Background())
	if err != nil || len(idle) != 3 {
		t.Fatalf("idle sessions: %+v, %v", idle, err)
	}
	for i, session := range idle {
		if session.State != agent.StateIdle || session.ID != sessions[i].ID {
			t.Fatalf("lost identity at completion: %+v", session)
		}
	}
}

func TestSessionHistoryDoesNotAssignOtherInstancesToPane(t *testing.T) {
	records := []sessionRecord{fixtureRecord("ses_one"), fixtureRecord("ses_two"), fixtureRecord("ses_history"), fixtureRecord("ses_child"), fixtureRecord("ses_archived"), fixtureRecord("ses_elsewhere")}
	records[3].ParentID = "ses_one"
	records[4].Time.Archived = 10
	records[5].Directory = "/elsewhere"
	// Status belongs to the HTTP instance; session history is shared by both.
	servers := make([]*httptest.Server, 2)
	for i, id := range []string{"ses_one", "ses_two"} {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/session":
				json.NewEncoder(w).Encode(records)
			case "/session/status":
				json.NewEncoder(w).Encode(map[string]statusRecord{id: {Type: "busy"}, "ses_child": {Type: "busy"}})
			default:
				w.Write([]byte("[]"))
			}
		}))
		defer servers[i].Close()
	}
	adapter := NewAdapter()
	instances := []instance{{pid: 10, directory: "/project", endpoint: servers[0].URL}, {pid: 20, directory: "/project", endpoint: servers[1].URL}}
	adapter.discover = func(context.Context) ([]instance, error) { return instances, nil }
	sessions, err := adapter.Sessions(context.Background())
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions: %+v, %v", sessions, err)
	}
	if sessions[0].TranscriptID != "ses_one" || *sessions[0].ProcessID != 10 || sessions[1].TranscriptID != "ses_two" || *sessions[1].ProcessID != 20 {
		t.Fatalf("wrong ownership: %+v", sessions)
	}
	// Exited processes disappear; reused PIDs must not inherit their old session.
	instances = []instance{{pid: 10, directory: "/project", started: time.Unix(500, 0)}}
	sessions, err = adapter.Sessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].State != agent.StateUnavailable || sessions[0].TranscriptID != "" {
		t.Fatalf("PID reuse inherited history: %+v, %v", sessions, err)
	}
	instances = nil
	sessions, err = adapter.Sessions(context.Background())
	if err != nil || len(sessions) != 0 {
		t.Fatalf("exited process retained: %+v, %v", sessions, err)
	}
}

func TestResumeAndDraftSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session":
			json.NewEncoder(w).Encode([]sessionRecord{fixtureRecord("ses_saved")})
		case "/session/status":
			w.Write([]byte("{}"))
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer server.Close()
	for _, entry := range []instance{
		{pid: 1, directory: "/project"},
		{pid: 2, directory: "/project", sessionID: "ses_saved"},
		{pid: 3, directory: "/project", continuing: true},
	} {
		adapter := fixtureAdapter(server, entry)
		sessions, err := adapter.Sessions(context.Background())
		if err != nil || len(sessions) != 1 || sessions[0].State != agent.StateIdle {
			t.Fatalf("idle instance: %+v, %v", sessions, err)
		}
		if entry.pid == 1 && sessions[0].TranscriptID != "" {
			t.Fatal("new instance adopted saved history")
		}
		if entry.pid != 1 && sessions[0].TranscriptID != "ses_saved" {
			t.Fatal("explicit resume omitted")
		}
	}
}

func TestAPIFailureRetainsSessionAndOtherInstances(t *testing.T) {
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/session":
			json.NewEncoder(w).Encode([]sessionRecord{fixtureRecord("ses_one")})
		case "/session/status":
			w.Write([]byte(`{"ses_one":{"type":"busy"}}`))
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer server.Close()
	adapter := fixtureAdapter(server, instance{pid: 1, directory: "/project"})
	adapter.discover = func(context.Context) ([]instance, error) {
		return []instance{{pid: 1, directory: "/project", endpoint: server.URL}, {pid: 2, directory: "/project"}}, nil
	}
	sessions, err := adapter.Sessions(context.Background())
	if err != nil || len(sessions) != 2 || sessions[1].State != agent.StateUnavailable {
		t.Fatalf("partial discovery: %+v, %v", sessions, err)
	}
	id := sessions[0].ID
	// Return values belong to the caller, not the adapter cache.
	sessions[0].TranscriptID = "corrupted"
	fail = true
	sessions, err = adapter.Sessions(context.Background())
	if err != nil || sessions[0].ID != id || sessions[0].TranscriptID != "ses_one" || sessions[0].State != agent.StateUnavailable {
		t.Fatalf("failure lost metadata: %+v, %v", sessions, err)
	}
}

func TestAPIValidationAuthenticationAndCancellation(t *testing.T) {
	t.Setenv("OPENCODE_SERVER_PASSWORD", "test-password")
	t.Setenv("OPENCODE_SERVER_USERNAME", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "opencode" || pass != "test-password" {
			t.Error("missing native auth")
		}
		switch r.URL.Path {
		case "/invalid":
			w.Write([]byte("not-json"))
		case "/null":
			w.Write([]byte(" null \n"))
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/redirect":
			http.Redirect(w, r, "https://example.com", http.StatusFound)
		case "/blocked":
			<-r.Context().Done()
		default:
			w.Write([]byte("{}"))
		}
	}))
	defer server.Close()
	adapter := NewAdapter()
	for _, path := range []string{"/invalid", "/null", "/unauthorized", "/redirect"} {
		var value map[string]any
		if err := adapter.get(context.Background(), server.URL, "/project", path, &value); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	for _, endpoint := range []string{"http://192.0.2.1:80", "https://127.0.0.1:80", "http://user:pass@127.0.0.1:80", server.URL + "/bad"} {
		if err := adapter.get(context.Background(), endpoint, "/project", "/session", new(any)); err == nil {
			t.Fatalf("accepted endpoint %s", endpoint)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := adapter.get(ctx, server.URL, "/project", "/blocked", new(any))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestDiscoveryParsers(t *testing.T) {
	lines := []string{
		"10 Sat Oct 10 13:36:09 2026 /tools/opencode /project --port 0 --session ses_one",
		"20 Sat Oct 10 13:36:09 2026 node /tools/opencode.exe --continue",
		"30 Sat Oct 10 13:36:09 2026 /tools/opencode session list",
		"40 Sat Oct 10 13:36:09 2026 sh -c opencode",
		"50 Sat Oct 10 13:36:09 2026 /tools/opencode --session=ses_two",
		"invalid output",
	}
	entries := parseProcesses(strings.Join(lines, "\n"))
	if len(entries) != 3 || entries[0].sessionID != "ses_one" || !entries[1].continuing || entries[2].sessionID != "ses_two" {
		t.Fatalf("processes: %+v", entries)
	}
	files := parseOpenFiles("p10\nfcwd\nn/project's directory\np20\nf5\nn127.0.0.1:5000\n")
	if files[10][0] != "/project's directory" || files[20][0] != "127.0.0.1:5000" {
		t.Fatalf("files: %v", files)
	}
}

func TestSlowInstancesDoNotPreventHealthyStatusReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("directory") != "/project" {
			<-r.Context().Done()
			return
		}
		switch r.URL.Path {
		case "/session":
			json.NewEncoder(w).Encode([]sessionRecord{fixtureRecord("ses_healthy")})
		case "/session/status":
			w.Write([]byte(`{"ses_healthy":{"type":"busy"}}`))
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer server.Close()
	adapter := fixtureAdapter(server,
		instance{pid: 1, directory: "/blocked-one"}, instance{pid: 2, directory: "/blocked-two"},
		instance{pid: 3, directory: "/blocked-three"}, instance{pid: 4, directory: "/project"},
	)
	adapter.client.Timeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	sessions, err := adapter.Sessions(ctx)
	if err != nil || len(sessions) != 4 {
		t.Fatalf("slow servers blocked provider poll: %+v, %v", sessions, err)
	}
	for _, session := range sessions[:3] {
		if session.State != agent.StateUnavailable {
			t.Fatalf("slow server reported healthy: %+v", session)
		}
	}
	if sessions[3].State != agent.StateRunning || sessions[3].TranscriptID != "ses_healthy" {
		t.Fatalf("healthy session lost: %+v", sessions[3])
	}
}
