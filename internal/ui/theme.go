package ui

import (
	"hash/fnv"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

const (
	colorBackground = "#0D1117"
	colorMainText   = "#D7DEE8"
	colorSecondary  = "#8996AA"
	colorAccent     = "#69D3E7"
	colorSelection  = "#203949"
	colorAgent      = "#BB9AF7"
	colorRunning    = "#9ECE6A"
	colorAttention  = "#E0AF68"
	colorError      = "#F7768E"
	colorDivider    = "#293442"
)

var (
	appStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			Padding(0, 1)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorMainText)).
			Background(lipgloss.Color(colorBackground)).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMainText)).
			Background(lipgloss.Color(colorBackground)).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Padding(1, 1, 0, 1).
			Width(sidebarContentWidth)
	panelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMainText)).
			Background(lipgloss.Color(colorBackground)).
			Padding(0, 1)
	detailStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Padding(1, 1, 0, 1)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary))
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorMainText))
	accentStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorAccent))
	mainTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMainText))
	shortcutKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorAccent))
	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRunning))
	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAttention))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError))
	agentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAgent))
	footerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Foreground(lipgloss.Color(colorSecondary)).
			Padding(0, 1)
)

func (model Model) panelTitleStyle() lipgloss.Style {
	if model.sidebarFocus {
		return sectionStyle
	}
	return accentStyle
}

func statusStyle(state agent.State) lipgloss.Style {
	switch state {
	case agent.StateRunning:
		return runningStyle
	case agent.StateWaiting:
		return waitingStyle
	case agent.StateError:
		return errorStyle
	case agent.StateUnavailable:
		return waitingStyle
	default:
		return mutedStyle
	}
}

func displayState(state agent.State) string {
	if state == "" {
		return "Unavailable"
	}

	return strings.ToUpper(string(state[:1])) + string(state[1:])
}

// providerStyle keeps each provider's color consistent across the dashboard.
func providerStyle(provider string) lipgloss.Style {
	color := colorAgent
	switch strings.ToLower(provider) {
	case "codex":
	case "claude":
		color = "#FFAF87"
	default:
		palette := []string{colorAccent, "#7DCFFF", "#73DACA", "#E0AF68", "#F7768E"}
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(strings.ToLower(provider)))
		color = palette[int(hash.Sum32())%len(palette)]
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
