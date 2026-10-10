package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const noticeDuration = 3 * time.Second

func shortcut(key, label string) string {
	keycap := lipgloss.NewStyle().Foreground(lipgloss.Color(colorMainText)).Background(lipgloss.Color(colorSection)).Padding(0, 1).Render(key)
	return keycap + mutedStyle.Render(" "+label)
}

func (model Model) footerText() string {
	if model.projectSearching {
		return strings.Join([]string{shortcut("enter", "select project"), shortcut("esc", "cancel"), shortcut("↑/↓", "browse")}, "   ")
	}
	parts := []string{shortcut("tab", "focus")}
	if model.navigator != nil {
		parts = append(parts, shortcut("enter", "open"))
	}
	parts = append(parts, shortcut("q", "quit"))
	left := strings.Join(parts, "   ")
	right := shortcut("?", "help")
	width := model.width - appStyle.GetHorizontalFrameSize() - footerStyle.GetHorizontalFrameSize()
	if model.width == 0 {
		width = lipgloss.Width(left) + lipgloss.Width(right) + 3
	}
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > width {
		left = strings.Join(parts, " ")
	}
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > width {
		keys := []string{"tab"}
		if model.navigator != nil {
			keys = append(keys, "enter")
		}
		keys = append(keys, "q")
		left = strings.Join(keys, " ")
		if lipgloss.Width(left)+2 > width {
			left = strings.Replace(left, "enter", "↵", 1)
		}
		left = shortcutKeyStyle.Render(left)
		right = shortcutKeyStyle.Render("?")
	}
	return alignedLine(left, right, max(1, width))
}

func (model Model) footerView(width int) string {
	if model.notice == "" {
		return footerStyle.Width(width).Render(clipLines(model.footerText(), width-footerStyle.GetHorizontalFrameSize()))
	}

	messageWidth := max(0, width-4)
	message := runewidth.Truncate(model.notice, messageWidth, "…")
	return footerStyle.Width(width).Render(errorStyle.Render("! " + message))
}

func clearNotice(revision int) tea.Cmd {
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg {
		return noticeExpiredMessage{revision: revision}
	})
}
