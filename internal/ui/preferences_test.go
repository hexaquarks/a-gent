package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDashboardLayoutPersistsAcrossLaunches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "dashboard.json")
	model := NewModel(nil, WithDashboardPreferences(path))
	if model.boardVisible || model.notice != "" {
		t.Fatal("first launch should use List without a load error")
	}

	for _, wantBoard := range []bool{true, false, false, true} {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
		model = updated.(Model)
		reloaded := NewModel(nil, WithDashboardPreferences(path))
		if model.boardVisible != wantBoard || reloaded.boardVisible != wantBoard || reloaded.notice != "" {
			t.Fatal("reopening did not restore the last chosen layout")
		}
		model = reloaded
	}

	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 || files[0].Name() != "dashboard.json" {
		t.Fatal("saving left temporary preference files")
	}
}

func TestInvalidDashboardPreferencesKeepListUsable(t *testing.T) {
	for _, data := range []string{"broken", `{"layout":"unknown"}`, `null`} {
		path := filepath.Join(t.TempDir(), "dashboard.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		model := NewModel(nil, WithDashboardPreferences(path))
		if model.boardVisible || model.notice == "" {
			t.Fatal("invalid preferences should fall back to List with a notice")
		}
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
		model = updated.(Model)
		if reloaded := NewModel(nil, WithDashboardPreferences(path)); !reloaded.boardVisible {
			t.Fatal("choosing a layout did not repair invalid preferences")
		}
	}
}

func TestLayoutSaveFailureStillSwitchesView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	model := NewModel(nil, WithDashboardPreferences(filepath.Join(path, "dashboard.json")))
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	model = updated.(Model)
	if !model.boardVisible || model.notice == "" || command == nil {
		t.Fatal("save failure should report a notice while switching to Board")
	}
}

func TestGraphPreferenceRestoresAndCyclesToList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	model := NewModel(nil, WithDashboardPreferences(path))
	model = sendProjectKey(model, runeKey('m'))
	model = sendProjectKey(model, runeKey('m'))
	restored := NewModel(nil, WithDashboardPreferences(path))
	if !restored.graphVisible || restored.boardVisible || restored.notice != "" {
		t.Fatal("Graph preference was not restored")
	}
	restored = sendProjectKey(restored, runeKey('m'))
	if restored.graphVisible || restored.boardVisible {
		t.Fatal("Graph should cycle to List")
	}
	restored = NewModel(nil, WithDashboardPreferences(path))
	if restored.graphVisible || restored.boardVisible {
		t.Fatal("List preference was not saved after Graph")
	}
}
