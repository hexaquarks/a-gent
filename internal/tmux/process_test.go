package tmux

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestReadProcessParentsSkipsMalformedRows(t *testing.T) {
	isolateCommand(t, "ps", `printf '%s\n' \
    '  100 1 /bin/sh -l' \
    '  200 100 /tools/codex --model fixture' \
    '' \
    'missing columns' \
    'invalid 100 /tools/claude' \
    '300 invalid /tools/pi'
`)

	parents, err := readProcessParents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]int{100: 1, 200: 100}
	if !reflect.DeepEqual(parents, want) {
		t.Fatalf("parents = %#v, want %#v", parents, want)
	}
}

func TestReadProcessTTYUsesProcessIDAndTrimsOutput(t *testing.T) {
	isolateCommand(t, "ps", `if [ "$#" != 4 ] || [ "$1" != '-p' ] || [ "$2" != '321' ] ||
    [ "$3" != '-o' ] || [ "$4" != 'tty=' ]; then
    exit 9
fi
printf '  /dev/ttys007  \n'
`)

	terminal, err := readProcessTTY(context.Background(), 321)
	if err != nil {
		t.Fatal(err)
	}
	if terminal != "/dev/ttys007" {
		t.Fatalf("terminal = %q", terminal)
	}
}

func TestProcessReadersPreserveCommandErrors(t *testing.T) {
	isolateCommand(t, "ps", "exit 6\n")

	processes, err := readProcesses(context.Background())
	assertProcessReadError(t, err)
	if processes != nil {
		t.Fatalf("failed process read returned %#v", processes)
	}

	parents, err := readProcessParents(context.Background())
	assertProcessReadError(t, err)
	if parents != nil {
		t.Fatalf("failed parent read returned %#v", parents)
	}

	terminal, err := readProcessTTY(context.Background(), 321)
	var exitError *exec.ExitError
	if terminal != "" || !errors.As(err, &exitError) || exitError.ExitCode() != 6 {
		t.Fatalf("terminal read = (%q, %v), want empty terminal and exit status 6", terminal, err)
	}
}

func assertProcessReadError(t *testing.T, err error) {
	t.Helper()

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 6 {
		t.Fatalf("process read error = %v, want exit status 6", err)
	}
	if !strings.Contains(err.Error(), "read session processes:") {
		t.Fatalf("process read error lacks context: %v", err)
	}
}

func TestExternalCommandsRespectCanceledContext(t *testing.T) {
	isolateCommand(t, "tmux", "exit 9\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runTmuxCommand(ctx, "list-panes", "-a")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("tmux command error = %v, want context cancellation", err)
	}

	// Use a harmless ps fixture even if cancellation regresses.
	isolateCommand(t, "ps", "exit 9\n")
	_, err = readProcessParents(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parent read error = %v, want context cancellation", err)
	}

	_, err = readProcessTTY(ctx, 321)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("terminal read error = %v, want context cancellation", err)
	}
}
