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
	parts := []string{shortcut("tab", "focus"), shortcut("j/k", "browse")}
	if model.navigator != nil {
		parts = append(parts, shortcut("enter", "open"))
	}
	holdLabel := "hold order"
	if model.orderHeld {
		holdLabel = "live order"
	}
	parts = append(parts, shortcut("s", "sort"), shortcut("f", holdLabel), shortcut("v", "preview"), shortcut("q", "quit"))
	width := model.width - appStyle.GetHorizontalFrameSize() - footerStyle.GetHorizontalFrameSize()
	if model.width > 0 && lipgloss.Width(strings.Join(parts, "   ")) > width {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts, "   ")
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
