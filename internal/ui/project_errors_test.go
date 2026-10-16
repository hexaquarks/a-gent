package ui

import (
	"os"
	"path/filepath"
	"testing"

	"a-gent/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInvalidProjectPinsKeepProjectsUsable(t *testing.T) {
	for _, data := range []string{"broken", `{"project":"not a boolean"}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pins.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}

			model := projectTestModel()
			WithProjectPins(path)(&model)
			if model.notice != "Could not load project pins" || len(model.projects()) != 80 {
				t.Fatalf("invalid pins hid projects or lost notice: %q", model.notice)
			}
		})
	}
}

func TestProjectSearchNavigationWrapsAndAcceptsSpaces(t *testing.T) {
	model := projectTestModel()
	model.sidebarFocus = true
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	start := model.projectItemStart()

	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyUp})
	if model.sidebarCursor != start+len(model.projects())-1 {
		t.Fatal("up at the first project did not wrap to the last")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	if model.sidebarCursor != start {
		t.Fatal("down at the last project did not wrap to the first")
	}

	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeySpace})
	if model.projectQuery != " " || len(model.matchingProjects()) != 0 {
		t.Fatal("space was not included in the search query")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyDown})
	if model.sidebarCursor != start {
		t.Fatal("navigation moved the cursor when no projects matched")
	}

	_, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if command == nil {
		t.Fatal("search swallowed the quit shortcut")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("quit shortcut did not request application shutdown")
	}
}

func TestProjectFilterClearsOnlyWhenItsLastSessionDisappears(t *testing.T) {
	model := projectTestModel()
	project := model.sessions[0].WorkingDirectory
	model.selectedProject = project
	model.selectedView = recentView

	model.applyProviderUpdate(polling.Update{Provider: "claude"})
	if model.selectedProject != project || model.selectedView != recentView {
		t.Fatal("an unrelated provider update cleared the project filter")
	}

	model.applyProviderUpdate(polling.Update{Provider: "codex"})
	if model.selectedProject != "" || model.selectedView != allView {
		t.Fatal("a vanished project left an empty, filtered dashboard")
	}
}

func TestDashboardSettingsReportFilesystemFailures(t *testing.T) {
	for _, setting := range []struct {
		name string
		load func(string) ModelOption
		save func(Model) error
	}{
		{name: "project pins", load: WithProjectPins, save: Model.saveProjectPins},
		{name: "layout", load: WithDashboardPreferences, save: Model.saveDashboardPreferences},
	} {
		t.Run(setting.name, func(t *testing.T) {
			root := t.TempDir()
			blocked := filepath.Join(root, "file")
			if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}

			model := NewModel(nil, setting.load(filepath.Join(blocked, "settings.json")))
			if model.notice == "" {
				t.Fatal("unreadable settings did not produce a notice")
			}
			if err := setting.save(model); err == nil {
				t.Fatal("saving through a file used as a directory succeeded")
			}

			// Renaming over a directory must fail without leaving a temporary settings file.
			destination := filepath.Join(root, "settings")
			if err := os.Mkdir(destination, 0o700); err != nil {
				t.Fatal(err)
			}
			model = NewModel(nil, setting.load(destination))
			if err := setting.save(model); err == nil {
				t.Fatal("saving over a directory succeeded")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 {
				t.Fatalf("failed save left temporary files: %v", entries)
			}
		})
	}
}
