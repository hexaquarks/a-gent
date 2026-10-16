package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableObserverRequiresInstalledCLIAndReportsPluginConflicts(t *testing.T) {
	for _, test := range []struct {
		name      string
		installed bool
		conflict  bool
	}{
		{name: "CLI unavailable"},
		{name: "installed CLI", installed: true},
		{name: "user plugin conflict", installed: true, conflict: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
			directory := filepath.Join(root, "configuration")
			t.Setenv("PI_CODING_AGENT_DIR", directory)

			binaries := t.TempDir()
			t.Setenv("PATH", binaries)
			if test.installed {
				executable := filepath.Join(binaries, "pi")
				if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}

			pluginPath := filepath.Join(directory, "extensions/a-gent.ts")
			const userPlugin = "// user-owned plugin\n"
			if test.conflict {
				if err := os.MkdirAll(filepath.Dir(pluginPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(pluginPath, []byte(userPlugin), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			adapter := NewAdapter()
			if adapter.Provider() != "pi" {
				t.Fatalf("provider = %q", adapter.Provider())
			}
			adapter.EnableObserver()

			if test.conflict {
				if adapter.setupErr == nil || !strings.Contains(adapter.setupErr.Error(), "refusing to replace") {
					t.Fatalf("setup error = %v", adapter.setupErr)
				}
				sessions, err := adapter.Sessions(context.Background())
				if sessions != nil || !errors.Is(err, adapter.setupErr) {
					t.Fatalf("sessions = %#v, error = %v; want setup error", sessions, err)
				}
				data, err := os.ReadFile(pluginPath)
				if err != nil || string(data) != userPlugin {
					t.Fatalf("user plugin changed: %q, %v", data, err)
				}
				return
			}

			if adapter.setupErr != nil {
				t.Fatal(adapter.setupErr)
			}
			data, err := os.ReadFile(pluginPath)
			if !test.installed {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("observer installed without CLI: %v", err)
				}
				return
			}
			if err != nil || len(data) == 0 {
				t.Fatalf("installed observer = %q, %v", data, err)
			}
		})
	}
}
