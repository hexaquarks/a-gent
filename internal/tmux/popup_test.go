package tmux

import (
	"strings"
	"testing"
)

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
