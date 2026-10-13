package hermes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func fixtureAdapter(t *testing.T) (Adapter, *runtimeRecord, *[]instance) {
	t.Helper()
	now := time.Unix(2000, 0)
	instances := []instance{{pid: 42, started: time.Unix(1000, 0)}}
	adapter := Adapter{directory: t.TempDir(), now: func() time.Time { return now },
		discover: func(context.Context) ([]instance, error) { return instances, nil }}
	record := &runtimeRecord{Version: 1, PID: 42, StartedAt: 1000000, UpdatedAt: now.UnixMilli(),
		LastActiveAt: 1999000, Directory: "/project", SessionID: "session-one", Name: "Hermes fixture", State: agent.StateRunning}
	writeRuntime(t, adapter, record)
	return adapter, record, &instances
}

func writeRuntime(t *testing.T, adapter Adapter, record *runtimeRecord) {
	t.Helper()
	directory := filepath.Join(adapter.directory, "instances")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "42.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSessionsNativeStatusIdentityAndProcessLifetime(t *testing.T) {
	adapter, record, instances := fixtureAdapter(t)
	var id string
	for _, state := range []agent.State{agent.StateRunning, agent.StateWaiting, agent.StateIdle, agent.StateRunning} {
		record.State = state
		writeRuntime(t, adapter, record)
		sessions, err := adapter.Sessions(context.Background())
		if err != nil || len(sessions) != 1 {
			t.Fatalf("sessions: %+v, %v", sessions, err)
		}
		entry := sessions[0]
		if entry.State != state || entry.Provider != "hermes" || *entry.ProcessID != 42 ||
			entry.WorkingDirectory != "/project" || entry.LastActiveAt.UnixMilli() != record.LastActiveAt {
			t.Fatalf("bad metadata: %+v", entry)
		}
		if id != "" && entry.ID != id {
			t.Fatal("status transition changed identity")
		}
		id = entry.ID
		*entry.ProcessID = 100 // Returned sessions do not mutate discovery metadata.
	}
	record.SessionID = "session-two"
	writeRuntime(t, adapter, record)
	sessions, _ := adapter.Sessions(context.Background())
	if sessions[0].ID == id {
		t.Fatal("switching sessions kept the old identity")
	}
	(*instances)[0].started = time.Unix(1100, 0)
	sessions, _ = adapter.Sessions(context.Background())
	if sessions[0].State != agent.StateUnavailable || sessions[0].TranscriptID != "" {
		t.Fatal("reused PID inherited old runtime metadata")
	}
	*instances = nil
	sessions, err := adapter.Sessions(context.Background())
	if err != nil || len(sessions) != 0 {
		t.Fatal("saved observer file appeared as a live instance")
	}
}

func TestSessionsRejectMissingStaleAndUnknownStatus(t *testing.T) {
	for _, kind := range []string{"missing", "stale", "future", "wrong-pid", "wrong-version", "unknown", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			adapter, record, _ := fixtureAdapter(t)
			switch kind {
			case "stale":
				record.UpdatedAt -= 6000
			case "future":
				record.UpdatedAt += 6000
			case "wrong-pid":
				record.PID++
			case "wrong-version":
				record.Version++
			case "unknown":
				record.State = "future-state"
			}
			writeRuntime(t, adapter, record)
			path := filepath.Join(adapter.directory, "instances", "42.json")
			if kind == "missing" {
				os.Remove(path)
			}
			if kind == "malformed" {
				os.WriteFile(path, []byte("{"), 0600)
			}
			sessions, err := adapter.Sessions(context.Background())
			if err != nil || len(sessions) != 1 || sessions[0].State != agent.StateUnavailable {
				t.Fatalf("insufficient metadata reported live status: %+v, %v", sessions, err)
			}
		})
	}
	adapter, _, _ := fixtureAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Sessions(ctx); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestDiscoveryRecognizesHermesExecutablesAndExcludesUtilities(t *testing.T) {
	commands := []string{
		"/tools/hermes", "python3 /tools/bin/hermes", "python3 /tools/hermes-agent/cli.py",
		"/tools/hermes chat --model fixture", "/tools/hermes plugins enable a-gent", "/tools/hermes sessions list",
		"/tools/hermes --version", "sh -c hermes", "python3 /project/cli.py", "/tools/hermes-agent", "/tools/hermes gateway",
		"python3 -u -m hermes_cli.main --profile coder chat -v", "/tools/hermes --model fixture-model",
		"/tools/hermes -p coder plugins list", "/tools/hermes --profile=coder sessions list",
	}

	var lines []string
	for i, command := range commands {
		lines = append(lines, strings.Join([]string{
			strconv.Itoa(i + 1), "Sat Oct 10 13:36:09 2026", command}, " "))
	}
	instances := parseProcesses(strings.Join(lines, "\n"))
	var ids []int
	for _, instance := range instances {
		ids = append(ids, instance.pid)
	}
	if !reflect.DeepEqual(ids, []int{1, 2, 3, 4, 12, 13}) {
		t.Fatalf("Hermes processes: %+v", instances)
	}
}

func TestInstallObserverIsAtomicAndDoesNotRewriteIdenticalFile(t *testing.T) {
	directory := t.TempDir()
	path, err := installObserver(directory)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if _, err := installObserver(directory); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Fatal("unchanged observer was replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(observer) || after.Mode().Perm() != 0600 {
		t.Fatal("observer not installed privately")
	}
}

func TestStaleObserverKeepsSessionIdentityAndLastActivity(t *testing.T) {
	adapter, record, _ := fixtureAdapter(t)
	sessions, _ := adapter.Sessions(context.Background())
	id := sessions[0].ID

	record.UpdatedAt -= 6000
	writeRuntime(t, adapter, record)
	sessions, err := adapter.Sessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != id ||
		sessions[0].State != agent.StateUnavailable || sessions[0].LastActiveAt.UnixMilli() != record.LastActiveAt {
		t.Fatalf("stale observer lost session identity: %+v, %v", sessions, err)
	}
}

func TestSubagentsRequireLiveRootAndValidParentTree(t *testing.T) {
	adapter, record, instances := fixtureAdapter(t)

	child := runtimeRecord{SessionID: "child", ParentID: record.SessionID, Name: "Review", Directory: record.Directory, State: agent.StateRunning}
	nested := child
	nested.SessionID, nested.ParentID = "nested", "child"
	orphan := child
	orphan.SessionID, orphan.ParentID = "orphan", "unknown"
	cycle := child
	cycle.SessionID, cycle.ParentID = "cycle", "cycle"
	invalid := child
	invalid.SessionID, invalid.State = "invalid", "unknown"
	record.Children = []runtimeRecord{child, nested, child, orphan, cycle, invalid}

	writeRuntime(t, adapter, record)
	sessions, err := adapter.Sessions(t.Context())
	if err != nil || len(sessions) != 3 {
		t.Fatalf("sessions: %+v, %v", sessions, err)
	}

	for _, session := range sessions[1:] {
		if session.ParentID == "" || session.ProcessID == nil || *session.ProcessID != 42 {
			t.Fatalf("child metadata: %+v", session)
		}
	}

	record.UpdatedAt -= 6000
	writeRuntime(t, adapter, record)
	sessions, _ = adapter.Sessions(t.Context())
	if len(sessions) != 1 {
		t.Fatal("stale heartbeat exposed children")
	}

	*instances = nil
	sessions, _ = adapter.Sessions(t.Context())
	if len(sessions) != 0 {
		t.Fatal("dead process exposed children")
	}
}
