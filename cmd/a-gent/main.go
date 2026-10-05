package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"a-gent/internal/agent"
	"a-gent/internal/claude"
	"a-gent/internal/codex"
	"a-gent/internal/polling"
	"a-gent/internal/tmux"
	"a-gent/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "a-gent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	popupOpened, err := tmux.OpenPopupInTmux(ui.PopupHeight)
	if err != nil {
		return fmt.Errorf("open tmux popup: %w", err)
	}

	if popupOpened {
		return nil
	}

	adapters := []agent.Adapter{codex.NewAdapter(), claude.NewAdapter()}
	modelOptions := []ui.ModelOption{}
	if configDirectory, err := os.UserConfigDir(); err == nil {
		modelOptions = append(modelOptions, ui.WithProjectPins(filepath.Join(configDirectory, "a-gent", "project-pins.json")))
	}
	if navigator := tmux.NewNavigator(); navigator != nil {
		modelOptions = append(modelOptions, ui.WithSessionNavigator(navigator))
	}

	ctx, cancel := context.WithCancel(context.Background())
	modelOptions = append(modelOptions, ui.WithApplicationContext(ctx))
	// Workers wait for the UI to receive each result so updates cannot pile up.
	updates := make(chan polling.Update)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		polling.Run(ctx, adapters, updates)
	}()
	defer func() {
		cancel()
		<-stopped
	}()

	program := tea.NewProgram(
		ui.NewModel(updates, modelOptions...),
		tea.WithAltScreen(),
	)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("dashboard stopped unexpectedly: %w", err)
	}
	return nil
}
