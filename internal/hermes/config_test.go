package hermes

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigureObserverPreservesSettingsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HERMES_HOME", root)
	original := []byte(`# user comment
model:
  default: custom-model
plugins:
  enabled: [existing]
  disabled: [other]
  entries:
    existing:
      api_key: secret
unknown: keep
`)
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	if err := yaml.Unmarshal(original, &before); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	before["plugins"].(map[string]any)["enabled"] = []any{"existing", "a-gent"}
	if !reflect.DeepEqual(before, after) || !strings.Contains(string(data), "# user comment") {
		t.Fatalf("unrelated config changed: %s", data)
	}
	info, _ := os.Stat(path)
	if _, err := ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	next, _ := os.Stat(path)
	if !os.SameFile(info, next) || info.Mode().Perm() != 0600 {
		t.Fatal("replaced unchanged config or exposed settings")
	}
}

func TestEnablePluginRejectsMalformedOrDisabledConfiguration(t *testing.T) {
	for _, original := range []string{
		"[not, a, mapping]", "plugins: invalid", "plugins:\n  enabled: nope", "plugins:\n  disabled: [a-gent]",
		"plugins:\n  enabled: [1]", "plugins: {}\n---\nplugins: {}", "plugins: {enabled: [one], enabled: [two]}",
	} {
		t.Run(original, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "config.yaml")
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if err := enablePlugin(root); err == nil {
				t.Fatal("accepted invalid or disabled config")
			}
			data, _ := os.ReadFile(path)
			if string(data) != original {
				t.Fatal("overwrote rejected config")
			}
		})
	}
}

func TestConfigureObserverUsesHermesHomeAndActiveProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("HERMES_HOME", "")
	home := filepath.Join(root, ".hermes")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "active_profile"), []byte("coder\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := ConfigureObserver()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(home, "profiles", "coder")) {
		t.Fatal("installed outside active profile")
	}
	custom := filepath.Join(root, "custom")
	t.Setenv("HERMES_HOME", custom)
	path, err = ConfigureObserver()
	if err != nil || !strings.HasPrefix(path, custom) {
		t.Fatalf("custom home: %s, %v", path, err)
	}
}

func TestEnablePluginPreservesConfigurationSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "shared.yaml")
	if err := os.WriteFile(target, []byte("model: fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.yaml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := enablePlugin(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replaced config symlink")
	}
	data, _ := os.ReadFile(target)
	if !strings.Contains(string(data), "a-gent") {
		t.Fatal("did not update config target")
	}
}
