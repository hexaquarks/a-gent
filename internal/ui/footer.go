package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

const noticeDuration = 3 * time.Second

func (model Model) footerText() string {
	parts := []string{
		shortcutKeyStyle.Render("tab") + mutedStyle.Render(": focus"),
		shortcutKeyStyle.Render("j/k") + mutedStyle.Render(": browse"),
	}
	if model.projectSearching {
		return shortcutKeyStyle.Render("enter") + mutedStyle.Render(": select project  •  ") + shortcutKeyStyle.Render("esc") + mutedStyle.Render(": cancel  •  ") + shortcutKeyStyle.Render("↑/↓") + mutedStyle.Render(": browse")
	}
	if model.navigator != nil {
		parts = append(parts, shortcutKeyStyle.Render("enter")+mutedStyle.Render(": open workspace"))
	}
	parts = append(parts,
		shortcutKeyStyle.Render("s")+mutedStyle.Render(": sort"),
		shortcutKeyStyle.Render("S")+mutedStyle.Render(": reverse"),
		shortcutKeyStyle.Render("q")+mutedStyle.Render(": quit"),
	)

	return strings.Join(parts, "  •  ")
}

func (model Model) footerView(width int) string {
	if model.notice == "" {
		return footerStyle.Width(width).Render(model.footerText())
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
