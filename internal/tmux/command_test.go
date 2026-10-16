package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolateCommand replaces PATH so these tests cannot reach a user's tmux server or process tools.
func isolateCommand(t *testing.T, name, script string) {
	t.Helper()

	directory := t.TempDir()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", directory)
}

func recordedArguments(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func TestRunTmuxCommandPreservesArgumentsOutputAndExitStatus(t *testing.T) {
	for _, test := range []struct {
		name     string
		exitCode string
	}{
		{name: "success", exitCode: "0"},
		{name: "command failure", exitCode: "8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			argumentsPath := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("COMMAND_ARGUMENTS", argumentsPath)
			t.Setenv("COMMAND_EXIT", test.exitCode)
			isolateCommand(t, "tmux", `printf '%s\0' "$@" > "$COMMAND_ARGUMENTS"
printf '%%42\n'
exit "$COMMAND_EXIT"
`)

			output, err := runTmuxCommand(context.Background(), "display-message", "-p", "argument with spaces")
			if string(output) != "%42\n" {
				t.Fatalf("command output = %q", output)
			}
			if test.exitCode == "0" && err != nil {
				t.Fatal(err)
			}
			if test.exitCode != "0" {
				var exitError *exec.ExitError
				if !errors.As(err, &exitError) || exitError.ExitCode() != 8 {
					t.Fatalf("command error = %v, want exit status 8", err)
				}
			}

			wantArguments := []string{"display-message", "-p", "argument with spaces"}
			if got := recordedArguments(t, argumentsPath); !reflect.DeepEqual(got, wantArguments) {
				t.Fatalf("command arguments = %#v, want %#v", got, wantArguments)
			}
		})
	}
}
