package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReturnsAfterTmuxPopup(t *testing.T) {
	for _, test := range []struct {
		name string
		exit string
	}{
		{name: "popup opens", exit: "0"},
		{name: "popup fails", exit: "7"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			marker := filepath.Join(directory, "opened")
			t.Setenv("PATH", directory)
			t.Setenv("TMUX", "isolated-test")
			t.Setenv("A_GENT_TMUX_POPUP", "")
			t.Setenv("POPUP_MARKER", marker)
			t.Setenv("POPUP_EXIT", test.exit)
			// Replace tmux entirely so startup cannot touch a user's server or launch a dashboard.
			script := `#!/bin/sh
case "$1" in
  display-message) printf 'test-client\n' ;;
  display-popup) printf 'opened' > "$POPUP_MARKER"; exit "$POPUP_EXIT" ;;
  *) exit 99 ;;
esac
`
			if err := os.WriteFile(filepath.Join(directory, "tmux"), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}

			err := run()
			if test.exit == "0" && err != nil {
				t.Fatal(err)
			}
			if test.exit != "0" && (err == nil || !strings.Contains(err.Error(), "open tmux popup: exit status 7")) {
				t.Fatalf("startup error = %v", err)
			}
			data, readErr := os.ReadFile(marker)
			if readErr != nil || string(data) != "opened" {
				t.Fatalf("popup was not invoked: %q, %v", data, readErr)
			}
		})
	}
}
