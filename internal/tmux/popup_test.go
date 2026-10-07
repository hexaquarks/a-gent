package tmux

import (
	"os/exec"
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
