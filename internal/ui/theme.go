package ui

import (
	"hash/fnv"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	colorBackground  = "#0C1112"
	colorMainText    = "#EDF2F4"
	colorSecondary   = "#A7B3C5"
	colorAccent      = "#59DFF0"
	colorSelection   = "#183740"
	colorAgent       = "#BC80FF"
	colorRunning     = "#A3EF78"
	colorAttention   = "#FFCA64"
	colorError       = "#FF6D7A"
	colorSection     = "#202B2D"
	colorTableHeader = "#2D3B3E"
	colorDivider     = "#354548"
)

var (
	appStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorDivider))
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
			Padding(0, 1).
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
			Padding(0, 1)
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

	if state == agent.StateWaiting {
		return "Needs input"
	}
	return strings.ToUpper(string(state[:1])) + string(state[1:])
}

// providerStyle keeps each provider's color consistent across the dashboard.
func providerStyle(provider string) lipgloss.Style {
	color := colorAgent
	switch strings.ToLower(provider) {
	case "codex":
	case "claude":
		color = "#FF997D"
	default:
		palette := []string{colorAccent, "#7DCFFF", "#73DACA", "#FFCA64", "#FF6D7A"}
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(strings.ToLower(provider)))
		color = palette[int(hash.Sum32())%len(palette)]
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

// Section bars fill their panel, including the space after the heading.
func sectionBar(text string, width int, style lipgloss.Style) string {
	return style.Background(lipgloss.Color(colorSection)).Width(width).MaxWidth(width).Render(ansi.Truncate(text, max(1, width), "…"))
}
