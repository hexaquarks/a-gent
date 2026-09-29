// Package tmux opens a-gent in a tmux popup when tmux is available.
package tmux

import (
	"os"
	"os/exec"
	"strings"
)

const popupEnvironmentVariable = "A_GENT_TMUX_POPUP"

// OpenPopup opens the current program in a centered tmux popup.
// It returns false when a-gent is not running inside tmux or is already in a popup.
func OpenPopup() (bool, error) {
	if os.Getenv("TMUX") == "" || os.Getenv(popupEnvironmentVariable) == "1" {
		return false, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return false, err
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return false, err
	}

	command := popupCommand(executable, os.Args[1:])
	popup := exec.Command(
		"tmux",
		"display-popup",
		"-E",
		"-w",
		"80%",
		"-h",
		"80%",
		"-d",
		workingDirectory,
		"-T",
		"a-gent",
		command,
	)
	popup.Stdin = os.Stdin
	popup.Stdout = os.Stdout
	popup.Stderr = os.Stderr

	return true, popup.Run()
}

func popupCommand(executable string, arguments []string) string {
	commandParts := []string{
		popupEnvironmentVariable + "=1",
		shellQuote(executable),
	}

	for _, argument := range arguments {
		commandParts = append(commandParts, shellQuote(argument))
	}

	return strings.Join(commandParts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
