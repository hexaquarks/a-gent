package tmux

import (
	"os/exec"
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
