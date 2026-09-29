package main

import (
	"fmt"
	"os"

	"a-gent/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	program := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "a-gent stopped unexpectedly: %v\n", err)
		os.Exit(1)
	}
}
