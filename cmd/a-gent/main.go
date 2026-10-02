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
	popupOpened, err := tmux.OpenPopupInTmux()
	if err != nil {
		fmt.Fprintf(os.Stderr, "a-gent could not open the tmux popup: %v\n", err)
		os.Exit(1)
	}

	if popupOpened {
		return
	}

	adapters := []agent.Adapter{codex.NewAdapter()}
	program := tea.NewProgram(ui.NewModel(adapters), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "a-gent stopped unexpectedly: %v\n", err)
		os.Exit(1)
	}
}
