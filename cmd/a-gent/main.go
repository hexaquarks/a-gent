package main

import (
	"fmt"
	"os"

	"a-gent/internal/agent"
	"a-gent/internal/codex"
	"a-gent/internal/tmux"
	"a-gent/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	popupOpened, err := tmux.OpenPopupInTmux(ui.PopupHeight)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a-gent could not open the tmux popup: %v\n", err)
		os.Exit(1)
	}

	if popupOpened {
		return
	}

	adapters := []agent.Adapter{codex.NewAdapter()}
	modelOptions := []ui.ModelOption{}
	if navigator := tmux.NewNavigator(); navigator != nil {
		modelOptions = append(modelOptions, ui.WithSessionNavigator(navigator))
	}

	program := tea.NewProgram(
		ui.NewModel(adapters, modelOptions...),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "a-gent stopped unexpectedly: %v\n", err)
		os.Exit(1)
	}
}
