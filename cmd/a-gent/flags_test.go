package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandInfoExitsBeforeDashboardStartup(t *testing.T) {
	// There must be no attempt to discover providers or open a real tmux popup.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("TMUX", "isolated-test")
	t.Setenv("A_GENT_TMUX_POPUP", "")

	for _, test := range []struct {
		argument string
		want     string
	}{
		{argument: "--version", want: "a-gent " + version + "\n"},
		{argument: "-version", want: "a-gent " + version + "\n"},
		{argument: "--help", want: "Usage of a-gent:"},
		{argument: "-h", want: "Usage of a-gent:"},
	} {
		t.Run(test.argument, func(t *testing.T) {
			var output bytes.Buffer
			if err := run([]string{test.argument}, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), test.want) {
				t.Fatalf("output = %q, want %q", output.String(), test.want)
			}
		})
	}
}

func TestCommandInfoRejectsUnsupportedArguments(t *testing.T) {
	for _, argument := range []string{"--unknown", "unexpected"} {
		var output bytes.Buffer
		if handled, err := showCommandInfo([]string{argument}, &output); !handled || err == nil {
			t.Fatalf("argument %q: handled = %v, error = %v", argument, handled, err)
		}
	}

	if handled, err := showCommandInfo(nil, &bytes.Buffer{}); handled || err != nil {
		t.Fatalf("normal startup: handled = %v, error = %v", handled, err)
	}
}
