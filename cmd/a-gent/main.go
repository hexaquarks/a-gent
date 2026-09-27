package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mihailanghelici/a-gent/internal/ui"
)

func main() {
	program := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "a-gent stopped unexpectedly: %v\\n", err)
		os.Exit(1)
	}
}
