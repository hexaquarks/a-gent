package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

const daemonTestTimeout = 5 * time.Second

func TestConnectRejectsInvalidHandshakes(t *testing.T) {
	testCases := []struct {
		name     string
		response string
		want     string
	}{
		{name: "HTTP rejection", response: "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n", want: "403 Forbidden"},
		{name: "invalid HTTP", response: "invalid HTTP\r\n\r\n", want: "read WebSocket handshake"},
		{
			name:     "incorrect accept key",
			response: "HTTP/1.1 101 Switching Protocols\r\nSec-WebSocket-Accept: wrong\r\n\r\n",
			want:     "invalid WebSocket handshake",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			socket := startTestDaemon(t, func(connection net.Conn, reader *bufio.Reader) error {
				if _, err := http.ReadRequest(reader); err != nil {
					return err
				}
				_, err := fmt.Fprint(connection, testCase.response)
				return err
			})
			ctx, cancel := context.WithTimeout(context.Background(), daemonTestTimeout)
			defer cancel()

			client, err := connect(ctx, socket)
			if client != nil {
				client.Close()
				t.Fatal("invalid handshake returned a client")
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("connect error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestConnectReportsMissingSocket(t *testing.T) {
	_, err := connect(context.Background(), filepath.Join(t.TempDir(), "missing.sock"))
	if err == nil || !strings.Contains(err.Error(), "connect to Codex daemon") {
		t.Fatalf("connect error = %v", err)
	}
}

func TestDaemonSocketPathValidatesCommandOutput(t *testing.T) {
	testCases := []struct {
		name     string
		output   string
		exitCode string
		want     string
	}{
		{name: "running", output: `{"status":"running","socketPath":"/tmp/fixture.sock"}`},
		{name: "stopped", output: `{"status":"stopped","socketPath":"/tmp/fixture.sock"}`, want: "not running"},
		{name: "missing socket", output: `{"status":"running"}`, want: "not running"},
		{name: "malformed metadata", output: `{`, want: "decode Codex daemon details"},
		{name: "command failed", exitCode: "1", want: "read Codex daemon details"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			installTestCodexCommand(t, testCase.output)
			if testCase.exitCode != "" {
				t.Setenv("CODEX_TEST_EXIT", testCase.exitCode)
			}

			path, err := daemonSocketPath(context.Background())
			if testCase.want != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.want) {
					t.Fatalf("daemon error = %v, want %q", err, testCase.want)
				}
				return
			}
			if err != nil || path != "/tmp/fixture.sock" {
				t.Fatalf("daemon socket = %q, %v", path, err)
			}
		})
	}
}

func TestAdapterReadsLiveDaemonSessions(t *testing.T) {
	responses := []string{
		`{"id":1,"result":{}}`,
		`{"id":2,"result":{"data":["main","worker","closed","orphan"]}}`,
		`{"id":3,"result":{"thread":{"id":"main","name":"Parent","cwd":"/fixture","path":"/fixture/main.jsonl","status":{"type":"idle"}}}}`,
		`{"id":4,"result":{"thread":{"id":"worker","parentThreadId":"main","agentNickname":"Ada","status":{"type":"active"}}}}`,
		`{"id":5,"result":{"thread":{"id":"closed","status":{"type":"notLoaded"}}}}`,
		`{"id":6,"result":{"thread":{"id":"orphan","parentThreadId":"closed","status":{"type":"active"}}}}`,
	}
	socket := startRPCDaemon(t, responses)
	metadata, err := json.Marshal(daemonVersion{Status: "running", SocketPath: socket})
	if err != nil {
		t.Fatal(err)
	}
	installTestCodexCommand(t, string(metadata))
	ctx, cancel := context.WithTimeout(context.Background(), daemonTestTimeout)
	defer cancel()

	adapter := NewAdapter()
	if adapter.Provider() != "codex" {
		t.Fatalf("provider = %q", adapter.Provider())
	}
	sessions, err := adapter.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	parent, worker := sessions[0], sessions[1]
	if parent.ID != "main" || parent.Name != "Parent" || parent.State != agent.StateIdle ||
		parent.WorkingDirectory != "/fixture" || parent.TranscriptPath != "/fixture/main.jsonl" {
		t.Fatalf("parent = %+v", parent)
	}
	if worker.ID != "worker" || worker.ParentID != parent.ID || worker.Name != "Ada" || worker.State != agent.StateRunning {
		t.Fatalf("worker = %+v", worker)
	}
}

func TestAdapterPropagatesDaemonFailures(t *testing.T) {
	testCases := []struct {
		name      string
		responses []string
	}{
		{name: "initialize", responses: []string{`{"id":1,"error":{"code":-1,"message":"fixture failure"}}`}},
		{
			name:      "list loaded threads",
			responses: []string{`{"id":1,"result":{}}`, `{"id":2,"error":{"code":-1,"message":"fixture failure"}}`},
		},
		{
			name: "read thread",
			responses: []string{
				`{"id":1,"result":{}}`,
				`{"id":2,"result":{"data":["main"]}}`,
				`{"id":3,"error":{"code":-1,"message":"fixture failure"}}`,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			socket := startRPCDaemon(t, testCase.responses)
			metadata, err := json.Marshal(daemonVersion{Status: "running", SocketPath: socket})
			if err != nil {
				t.Fatal(err)
			}
			installTestCodexCommand(t, string(metadata))
			ctx, cancel := context.WithTimeout(context.Background(), daemonTestTimeout)
			defer cancel()

			sessions, err := NewAdapter().Sessions(ctx)
			if err == nil || !strings.Contains(err.Error(), "fixture failure") || sessions != nil {
				t.Fatalf("sessions = %+v, error = %v", sessions, err)
			}
		})
	}
}

func installTestCodexCommand(t *testing.T, output string) {
	t.Helper()
	const script = `#!/bin/sh
if [ "$*" != "app-server daemon version" ]; then
    exit 2
fi
printf '%s' "$CODEX_TEST_METADATA"
exit "${CODEX_TEST_EXIT:-0}"
`
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "codex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("CODEX_TEST_METADATA", output)
	t.Setenv("CODEX_TEST_EXIT", "0")
}

func startRPCDaemon(t *testing.T, responses []string) string {
	t.Helper()
	return startTestDaemon(t, func(connection net.Conn, reader *bufio.Reader) error {
		request, err := http.ReadRequest(reader)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(connection,
			"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			webSocketAccept(request.Header.Get("Sec-WebSocket-Key")),
		)
		if err != nil {
			return err
		}

		for _, response := range responses {
			for {
				_, payload, err := readMaskedClientFrame(reader)
				if err != nil {
					return err
				}
				var message struct {
					ID int `json:"id"`
				}
				if err := json.Unmarshal(payload, &message); err != nil {
					return err
				}
				if message.ID != 0 {
					break
				}
			}
			if _, err := connection.Write(serverFrame(testTextFrame, []byte(response))); err != nil {
				return err
			}
		}
		return nil
	})
}

func startTestDaemon(t *testing.T, serve func(net.Conn, *bufio.Reader) error) string {
	t.Helper()
	// Keep the socket outside t.TempDir's long paths to fit Unix socket limits.
	directory, err := os.MkdirTemp("/tmp", "codex-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	socket := filepath.Join(directory, "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), daemonTestTimeout)
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		stop := context.AfterFunc(ctx, func() { connection.Close() })
		defer stop()

		done <- serve(connection, bufio.NewReader(connection))
	}()
	t.Cleanup(func() {
		cancel()
		listener.Close()
		if err := <-done; err != nil && !t.Failed() {
			t.Errorf("fixture daemon: %v", err)
		}
	})
	return socket
}
