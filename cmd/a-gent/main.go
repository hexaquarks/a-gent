package main

import (
	"fmt"
	"os"

	"a-gent/internal/tmux"
	"a-gent/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	opened, err := tmux.OpenPopup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "a-gent could not open the tmux popup: %v\n", err)
		os.Exit(1)
	}

	if opened {
		return
	}

	program := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "a-gent stopped unexpectedly: %v\n", err)
		os.Exit(1)
	}
}
