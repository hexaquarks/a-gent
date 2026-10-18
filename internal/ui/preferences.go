package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type dashboardPreferences struct {
	Layout string `json:"layout"`
}

// WithDashboardPreferences restores the last layout from the supplied file.
func WithDashboardPreferences(path string) ModelOption {
	return func(model *Model) {
		model.preferencesPath = path
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return
		}

		var preferences dashboardPreferences
		if err == nil {
			err = json.Unmarshal(data, &preferences)
		}
		if err != nil || (preferences.Layout != "list" && preferences.Layout != "board" && preferences.Layout != "graph") {
			model.notice = "Could not load saved layout"
			return
		}

		model.boardVisible = preferences.Layout == "board"
		model.graphVisible = preferences.Layout == "graph"
	}
}

func (model Model) saveDashboardPreferences() error {
	if model.preferencesPath == "" {
		return nil
	}

	preferences := dashboardPreferences{Layout: "list"}
	if model.boardVisible {
		preferences.Layout = "board"
	}
	if model.graphVisible {
		preferences.Layout = "graph"
	}
	data, err := json.Marshal(preferences)
	if err != nil {
		return err
	}

	directory := filepath.Dir(model.preferencesPath)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}

	// Replace the file only after writing it completely, as with project pins.
	file, err := os.CreateTemp(directory, "dashboard-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write layout: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), model.preferencesPath)
}
