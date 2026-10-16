package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installDiscoveryCommands(t *testing.T, processes, directories, listeners string) string {
	t.Helper()
	directory := t.TempDir()
	log := filepath.Join(directory, "lsof-arguments")
	commands := map[string]string{
		"ps": `#!/bin/sh
if [ "$LC_ALL" != C ]; then exit 3; fi
printf '%s' "$A_GENT_TEST_PROCESSES"
exit "${A_GENT_TEST_PS_EXIT:-0}"
`,
		"lsof": `#!/bin/sh
printf '%s\n' "$*" >> "$A_GENT_TEST_COMMAND_LOG"
case " $* " in
  *" -d cwd "*)
    printf '%s' "$A_GENT_TEST_DIRECTORIES"
    exit "${A_GENT_TEST_CWD_EXIT:-0}"
    ;;
  *" -sTCP:LISTEN "*)
    printf '%s' "$A_GENT_TEST_LISTENERS"
    exit "${A_GENT_TEST_LISTENER_EXIT:-0}"
    ;;
  *) exit 4 ;;
esac
`,
	}
	for name, command := range commands {
		// These temporary files must be executable to exercise the OS command boundary.
		if err := os.WriteFile(filepath.Join(directory, name), []byte(command), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	t.Setenv("A_GENT_TEST_COMMAND_LOG", log)
	t.Setenv("A_GENT_TEST_PROCESSES", processes)
	t.Setenv("A_GENT_TEST_DIRECTORIES", directories)
	t.Setenv("A_GENT_TEST_LISTENERS", listeners)
	t.Setenv("A_GENT_TEST_PS_EXIT", "0")
	t.Setenv("A_GENT_TEST_CWD_EXIT", "0")
	t.Setenv("A_GENT_TEST_LISTENER_EXIT", "0")
	return log
}

func TestDiscoverInstancesUsesOnlyProcessOwnedLoopbackListeners(t *testing.T) {
	processes := "42 Mon Jan 2 15:04:05 2006 /usr/local/bin/opencode --session ses_one\n" +
		"43 Mon Jan 2 15:04:05 2006 node /opt/opencode --continue\n" +
		"44 Mon Jan 2 15:04:05 2006 opencode\n" +
		"45 Mon Jan 2 15:04:05 2006 opencode models\n"
	directories := "n/ignored-without-pid\np42\nn/projects/with spaces\nn/secondary\np43\nn/projects/ipv6\n"
	listeners := "p42\nn192.0.2.1:9000\nnnot-an-address\nn*:4100\n" +
		"p43\nn[::]:4200\np44\nn192.0.2.2:4300\n"
	log := installDiscoveryCommands(t, processes, directories, listeners)

	entries, err := discoverInstances(context.Background())
	if err != nil || len(entries) != 3 {
		t.Fatalf("discovery = %+v, %v", entries, err)
	}
	first, second, remote := entries[0], entries[1], entries[2]
	if first.pid != 42 || first.directory != "/projects/with spaces" || first.sessionID != "ses_one" ||
		first.endpoint != "http://127.0.0.1:4100" || first.started.Year() != 2006 {
		t.Fatalf("process metadata or IPv4 listener was lost: %+v", first)
	}
	if !second.continuing || second.directory != "/projects/ipv6" || second.endpoint != "http://[::1]:4200" {
		t.Fatalf("IPv6 wildcard did not resolve to loopback: %+v", second)
	}
	if remote.endpoint != "" || remote.directory != "" {
		t.Fatalf("remote listener or missing working directory was fabricated: %+v", remote)
	}
	arguments, err := os.ReadFile(log)
	expected := "-a -p 42,43,44 -d cwd -Fpn\n-nP -a -p 42,43,44 -iTCP -sTCP:LISTEN -Fpn\n"
	if err != nil || string(arguments) != expected {
		t.Fatalf("process-owned file queries = %q, %v", arguments, err)
	}
}

func TestDiscoverInstancesHandlesMissingListenersAndCommandFailures(t *testing.T) {
	const process = "42 Mon Jan 2 15:04:05 2006 opencode\n"
	for _, test := range []struct {
		name      string
		processes string
		variable  string
		exit      string
		wantError string
		wantCount int
	}{
		{name: "no processes skips lsof", variable: "A_GENT_TEST_CWD_EXIT", exit: "2"},
		{
			name: "ps failure", processes: process, variable: "A_GENT_TEST_PS_EXIT", exit: "2",
			wantError: "list OpenCode processes",
		},
		{name: "cwd failure", processes: process, variable: "A_GENT_TEST_CWD_EXIT", exit: "2", wantError: "requires lsof"},
		{
			name: "listener failure", processes: process, variable: "A_GENT_TEST_LISTENER_EXIT", exit: "2",
			wantError: "requires lsof",
		},
		{name: "cwd no matches is allowed", processes: process, variable: "A_GENT_TEST_CWD_EXIT", exit: "1", wantCount: 1},
		{name: "no listener is allowed", processes: process, variable: "A_GENT_TEST_LISTENER_EXIT", exit: "1", wantCount: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			installDiscoveryCommands(t, test.processes, "", "")
			t.Setenv(test.variable, test.exit)

			entries, err := discoverInstances(context.Background())
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) || len(entries) != 0 {
					t.Fatalf("discovery = %+v, %v; want %q", entries, err, test.wantError)
				}
				return
			}
			if err != nil || len(entries) != test.wantCount {
				t.Fatalf("discovery = %+v, %v; want %d instances", entries, err, test.wantCount)
			}
		})
	}
}

func TestDiscoverInstancesHonorsCancellation(t *testing.T) {
	installDiscoveryCommands(t, "", "", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entries, err := discoverInstances(ctx)
	if !errors.Is(err, context.Canceled) || len(entries) != 0 {
		t.Fatalf("cancelled discovery = %+v, %v", entries, err)
	}
}
