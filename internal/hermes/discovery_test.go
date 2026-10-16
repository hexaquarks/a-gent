package hermes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installDiscoveryCommands(t *testing.T, processes, directories string) string {
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
printf '%s' "$*" > "$A_GENT_TEST_COMMAND_LOG"
printf '%s' "$A_GENT_TEST_DIRECTORIES"
exit "${A_GENT_TEST_CWD_EXIT:-0}"
`,
	}
	for name, command := range commands {
		// These temporary files must be executable to exercise the OS command boundary.
		if err := os.WriteFile(filepath.Join(directory, name), []byte(command), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	t.Setenv("A_GENT_TEST_PROCESSES", processes)
	t.Setenv("A_GENT_TEST_DIRECTORIES", directories)
	t.Setenv("A_GENT_TEST_COMMAND_LOG", log)
	t.Setenv("A_GENT_TEST_PS_EXIT", "0")
	t.Setenv("A_GENT_TEST_CWD_EXIT", "0")
	return log
}

func TestDiscoverInstancesKeepsProcessIdentityAndWorkingDirectories(t *testing.T) {
	processes := "42 Mon Jan 2 15:04:05 2006 /usr/local/bin/hermes\n" +
		"43 Mon Jan 2 15:04:05 2006 hermes\n" +
		"44 Mon Jan 2 15:04:05 2006 hermes --help\n"
	directories := "n/ignored-without-pid\np42\nn/projects/with spaces\np99\nn/unrelated\npbad\nn/invalid\n"
	log := installDiscoveryCommands(t, processes, directories)

	entries, err := discoverInstances(context.Background())
	if err != nil || len(entries) != 2 {
		t.Fatalf("discovery = %+v, %v", entries, err)
	}
	first, second := entries[0], entries[1]
	if first.pid != 42 || first.directory != "/projects/with spaces" || first.started.Year() != 2006 {
		t.Fatalf("process metadata or working directory was lost: %+v", first)
	}
	if second.pid != 43 || second.directory != "" {
		t.Fatalf("unrelated cwd was assigned to a process with no matching files: %+v", second)
	}
	arguments, err := os.ReadFile(log)
	if err != nil || string(arguments) != "-a -p 42,43 -d cwd -Fpn" {
		t.Fatalf("working directory query = %q, %v", arguments, err)
	}
}

func TestDiscoverInstancesHandlesEmptyResultsAndCommandFailures(t *testing.T) {
	const process = "42 Mon Jan 2 15:04:05 2006 hermes\n"
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
			wantError: "list Hermes processes",
		},
		{
			name: "cwd failure", processes: process, variable: "A_GENT_TEST_CWD_EXIT", exit: "2",
			wantError: "requires lsof",
		},
		{
			name: "no matching working directory", processes: process,
			variable: "A_GENT_TEST_CWD_EXIT", exit: "1", wantCount: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			installDiscoveryCommands(t, test.processes, "")
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
	installDiscoveryCommands(t, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entries, err := discoverInstances(ctx)
	if !errors.Is(err, context.Canceled) || len(entries) != 0 {
		t.Fatalf("cancelled discovery = %+v, %v", entries, err)
	}
}
