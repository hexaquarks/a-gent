// Package tmux opens a-gent in a tmux popup when tmux is available.
package tmux

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const popupEnvironmentVariable = "A_GENT_TMUX_POPUP"

const popupTitle = "#[fg=#69D3E7,bold] a-gent "

// OpenPopupInTmux opens the current program in a centered tmux popup sized for
// the supplied number of terminal rows.
// It returns false when a-gent is not running inside tmux or is already in a popup.
func OpenPopupInTmux(popupHeight int) (bool, error) {
	if os.Getenv("TMUX") == "" || os.Getenv(popupEnvironmentVariable) == "1" {
		return false, nil
	}

	executablePath, err := os.Executable()
	if err != nil {
		return false, err
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return false, err
	}

	command := popupCommand(executablePath, os.Args[1:], activeClientName())
	popup := exec.Command("tmux", popupArguments(popupHeight, workingDirectory, command)...)
	popup.Stdin = os.Stdin
	popup.Stdout = os.Stdout
	popup.Stderr = os.Stderr

	return true, popup.Run()
}

func popupArguments(popupHeight int, workingDirectory, command string) []string {
	return []string{
		"display-popup",
		"-E",
		"-b",
		"rounded",
		"-s",
		"bg=#0D1117,fg=#D7DEE8",
		"-S",
		"fg=#293442",
		"-w",
		"70%",
		"-h",
		strconv.Itoa(popupHeight),
		"-d",
		workingDirectory,
		"-T",
		popupTitle,
		command,
	}
}

func activeClientName() string {
	output, err := exec.Command("tmux", "display-message", "-p", "#{client_name}").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// popupCommand preserves arguments when the popup relaunches the current executable.
func popupCommand(executable string, arguments []string, clientName string) string {
	commandParts := []string{
		popupEnvironmentVariable + "=1",
	}
	if clientName != "" {
		commandParts = append(commandParts, tmuxClientEnvironmentVariable+"="+shellQuote(clientName))
	}
	commandParts = append(commandParts, shellQuote(executable))

	for _, argument := range arguments {
		commandParts = append(commandParts, shellQuote(argument))
	}

	return strings.Join(commandParts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
