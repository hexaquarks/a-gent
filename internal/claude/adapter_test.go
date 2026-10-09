package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestDecodeSessions(t *testing.T) {
	sessions, err := decodeSessions([]byte(`[
 {"pid":42,"sessionId":"interactive","cwd":"/project","kind":"interactive","name":"Review","status":"waiting","startedAt":1791140769142},
 {"id":"job-1","sessionId":"conversation","kind":"background","state":"working"},
 {"id":"job-2","kind":"background","state":"blocked"},
 {"id":"closed","kind":"background","state":"done"},
 {"pid":43,"sessionId":"worker","kind":"subagent","status":"busy"}
 ]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("sessions = %+v", sessions)
	}
	first := sessions[0]
	if first.ID != "interactive" || first.Provider != "claude" || first.Name != "Review" || first.WorkingDirectory != "/project" || first.ProcessID == nil || *first.ProcessID != 42 || first.State != agent.StateWaiting {
		t.Fatalf("interactive session = %+v", first)
	}
	if !first.LastActiveAt.IsZero() {
		t.Fatal("start time was mistaken for activity")
	}
	if sessions[1].ID != "job:job-1" || sessions[1].State != agent.StateRunning || sessions[2].State != agent.StateWaiting {
		t.Fatalf("background sessions = %+v", sessions[1:])
	}
	if sessions[1].ProcessID == nil || *sessions[1].ProcessID != 0 {
		t.Fatal("processless background job must not navigate by project directory")
	}
}

func TestActivityMapping(t *testing.T) {
	for _, test := range []struct {
		status, state string
		want          agent.State
	}{
		{"busy", "", agent.StateRunning},
		{"waiting", "", agent.StateWaiting},
		{"idle", "", agent.StateIdle},
		{"future", "", agent.StateUnavailable},
		{"idle", "working", agent.StateRunning},
		{"waiting", "working", agent.StateWaiting},
		{"idle", "blocked", agent.StateWaiting},
		{"idle", "failed", agent.StateError},
		{"idle", "done", agent.StateIdle},
		{"idle", "stopped", agent.StateIdle},
	} {
		t.Run(test.status+"/"+test.state, func(t *testing.T) {
			if got := (session{Status: test.status, State: test.state}).activity(); got != test.want {
				t.Fatalf("state = %s, want %s", got, test.want)
			}
		})
	}
}

func TestSessionIdentityAcrossBackgroundRestart(t *testing.T) {
	var id string
	for _, payload := range []string{
		`[{"kind":"background","id":"job","state":"working"}]`,
		`[{"kind":"background","id":"job","state":"working","pid":42,"sessionId":"uuid"}]`,
	} {
		sessions, err := decodeSessions([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		if id != "" && id != sessions[0].ID {
			t.Fatal("restart changed identity")
		}
		id = sessions[0].ID
	}
}

func TestDecodeRejectsInvalidOutput(t *testing.T) {
	for _, payload := range []string{"", "null", "{}", "[", `[{"kind":"background","state":"working"}]`} {
		if _, err := decodeSessions([]byte(payload)); err == nil {
			t.Fatalf("accepted %q", payload)
		}
	}
	sessions, err := decodeSessions([]byte("[]"))
	if err != nil || len(sessions) != 0 {
		t.Fatalf("empty sessions: %v, %v", sessions, err)
	}
}

func TestAdapterInvokesReadOnlyCLI(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "claude")
	script := "#!/bin/sh\n[ \"$*\" = 'agents --json' ] || exit 2\nprintf '%s' '[{\"pid\":42,\"sessionId\":\"live\",\"kind\":\"interactive\",\"status\":\"idle\"}]'\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	sessions, err := NewAdapter().Sessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "live" {
		t.Fatalf("sessions = %v, error = %v", sessions, err)
	}

	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := NewAdapter().Sessions(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
	if time.Since(start) > time.Second {
		t.Fatal("subprocess did not stop on cancellation")
	}
}

func TestAdapterReportsCommandFailures(t *testing.T) {
	failure := errors.New("unsupported command")
	adapter := Adapter{run: func(context.Context) ([]byte, error) { return nil, failure }}
	_, err := adapter.Sessions(context.Background())
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "claude agents --json") {
		t.Fatalf("error = %v", err)
	}
}
