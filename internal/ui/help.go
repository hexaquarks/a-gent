package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func helpLines(width int) []string {
	bindings := [][2]string{
		{"↑/↓", "Move selection"},
		{"Tab", "Switch sidebar / sessions"},
		{"Enter", "Open / select"},
		{"/", "Find project"},
		{"p", "Pin / unpin project"},
		{"s", "Change sort column"},
		{"Shift+S", "Reverse sort order"},
		{"v", "Expand preview"},
		{"n", "New agent in pane"},
		{"Shift+N", "New agent in window"},
		{"q", "Quit dashboard"},
	}
	var lines []string
	for _, binding := range bindings {
		key := shortcutKeyStyle.Width(11).Render(binding[0])
		text := key + mainTextStyle.Render(binding[1])
		if width < 36 {
			text = shortcutKeyStyle.Render(binding[0]) + "\n" + mutedStyle.Render(binding[1])
		}
		wrapped := lipgloss.NewStyle().Width(width).Render(text)
		lines = append(lines, strings.Split(wrapped, "\n")...)
	}
	return lines
}

func (model Model) helpDimensions() (width, visibleLines int) {
	width = min(48, max(1, model.width-8))
	visibleLines = max(1, model.height-10)
	return width, visibleLines
}

func (model Model) updateHelp(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	width, visibleLines := model.helpDimensions()
	lastOffset := max(0, len(helpLines(width))-visibleLines)
	model.helpScroll = min(model.helpScroll, lastOffset)
	switch key.String() {
	case "q", "esc", "?":
		model.helpOpen = false
	case "ctrl+c":
		if model.previewCancel != nil {
			model.previewCancel()
		}
		model.cancelLaunch()
		return model.update(key)
	case "j", "down":
		model.helpScroll++
	case "k", "up":
		model.helpScroll--
	case "pgdown", " ":
		model.helpScroll += visibleLines
	case "pgup":
		model.helpScroll -= visibleLines
	case "home", "g":
		model.helpScroll = 0
	case "end", "G":
		model.helpScroll = lastOffset
	}
	model.helpScroll = max(0, min(model.helpScroll, lastOffset))
	return model, nil
}

func (model Model) helpView(background string) string {
	width, visibleLines := model.helpDimensions()
	lines := helpLines(width)
	start := max(0, min(model.helpScroll, len(lines)-visibleLines))
	end := min(len(lines), start+visibleLines)
	footer := "Esc/q close"
	if len(lines) > visibleLines {
		footer = "↑/↓ scroll · Esc/q close"
		if width < 30 {
			footer = "↑/↓ · Esc/q close"
		}
	}
	content := accentStyle.Render("KEYBOARD HELP") + "\n\n" +
		strings.Join(lines[start:end], "\n") + "\n\n" + mutedStyle.Render(clipLines(footer, width))
	box := lipgloss.NewStyle().Width(width+4).Padding(1, 2).
		Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(colorAccent)).
		Background(lipgloss.Color(colorBackground)).Render(content)
	return model.overlayPanel(background, box)
}
