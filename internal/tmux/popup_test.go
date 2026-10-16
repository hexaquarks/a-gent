package tmux

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestShellQuotePreservesLiteralArguments(t *testing.T) {
	for _, value := range []string{"", "path with spaces", "a'b", "$(printf injected)", "`printf injected`", "a; printf injected", "first\nsecond"} {
		t.Run(value, func(t *testing.T) {
			output, err := exec.Command("sh", "-c", "printf '%s' "+shellQuote(value)).Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(output) != value {
				t.Fatalf("quoted value changed: got %q, want %q", output, value)
			}
		})
	}
}

func TestPopupCommandPassesTheOriginatingClient(t *testing.T) {
	command := popupCommand("/usr/local/bin/a-gent", []string{"--verbose"}, "/dev/ttys001")

	for _, expected := range []string{
		popupEnvironmentVariable + "=1",
		tmuxClientEnvironmentVariable + "='/dev/ttys001'",
		"'/usr/local/bin/a-gent'",
		"'--verbose'",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("popup command %q does not contain %q", command, expected)
		}
	}
}

func TestPopupUsesFixedDimensionsAndTheApplicationBorder(t *testing.T) {
	actual := popupArguments(120, 28, "/projects/a-gent", "A_GENT_TMUX_POPUP=1 a-gent")
	expected := []string{
		"display-popup", "-E", "-B", "-s", "bg=#0C1112,fg=#EDF2F4",
		"-w", "120", "-h", "28", "-d", "/projects/a-gent", "-T", "", "A_GENT_TMUX_POPUP=1 a-gent",
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("popup arguments = %#v, want %#v", actual, expected)
	}
}

func TestOpenPopupBypassesTmuxOutsideSessionOrInsidePopup(t *testing.T) {
	for _, test := range []struct {
		name  string
		tmux  string
		popup string
	}{
		{name: "outside tmux"},
		{name: "already in popup", tmux: "fixture", popup: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TMUX", test.tmux)
			t.Setenv(popupEnvironmentVariable, test.popup)
			t.Setenv("PATH", t.TempDir())

			opened, err := OpenPopupInTmux(120, 28)
			if opened || err != nil {
				t.Fatalf("popup bypass returned (%v, %v), want (false, nil)", opened, err)
			}
		})
	}
}

func TestOpenPopupDispatchesArgumentsAndReportsCommandErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		clientExit string
		popupExit  string
		wantClient string
	}{
		{name: "originating client", clientExit: "0", popupExit: "0", wantClient: "/dev/fixture client"},
		{name: "client unavailable", clientExit: "1", popupExit: "0"},
		{name: "popup failure", clientExit: "0", popupExit: "7", wantClient: "/dev/fixture client"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TMUX", "fixture")
			t.Setenv(popupEnvironmentVariable, "")
			t.Setenv("CLIENT_EXIT", test.clientExit)
			t.Setenv("POPUP_EXIT", test.popupExit)

			clientArguments := filepath.Join(t.TempDir(), "client-arguments")
			popupArgumentsPath := filepath.Join(t.TempDir(), "popup-arguments")
			t.Setenv("CLIENT_ARGUMENTS", clientArguments)
			t.Setenv("POPUP_ARGUMENTS", popupArgumentsPath)
			isolateCommand(t, "tmux", `case "$1" in
    display-message)
        printf '%s\0' "$@" > "$CLIENT_ARGUMENTS"
        printf '  /dev/fixture client\n'
        exit "$CLIENT_EXIT"
        ;;
    display-popup)
        printf '%s\0' "$@" > "$POPUP_ARGUMENTS"
        exit "$POPUP_EXIT"
        ;;
    *) exit 99 ;;
esac
`)

			opened, err := OpenPopupInTmux(120, 28)
			if !opened {
				t.Fatal("popup dispatch did not report an attempted launch")
			}
			if test.popupExit == "0" && err != nil {
				t.Fatal(err)
			}
			if test.popupExit != "0" {
				var exitError *exec.ExitError
				if !errors.As(err, &exitError) || exitError.ExitCode() != 7 {
					t.Fatalf("popup error = %v, want exit status 7", err)
				}
			}

			wantClientArguments := []string{"display-message", "-p", "#{client_name}"}
			if got := recordedArguments(t, clientArguments); !reflect.DeepEqual(got, wantClientArguments) {
				t.Fatalf("client arguments = %#v, want %#v", got, wantClientArguments)
			}

			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			directory, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			wantCommand := popupCommand(executable, os.Args[1:], test.wantClient)
			wantArguments := popupArguments(120, 28, directory, wantCommand)
			if got := recordedArguments(t, popupArgumentsPath); !reflect.DeepEqual(got, wantArguments) {
				t.Fatalf("popup arguments = %#v, want %#v", got, wantArguments)
			}
		})
	}
}
