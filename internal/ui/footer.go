package ui

import (
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

const noticeDuration = 3 * time.Second

func (model Model) footerText() string {
	parts := []string{
		shortcutKeyStyle.Render("tab") + mutedStyle.Render(": switch focus"),
		shortcutKeyStyle.Render("j/k or ↑/↓") + mutedStyle.Render(": browse"),
	}
	if model.navigator != nil {
		parts = append(parts, shortcutKeyStyle.Render("enter")+mutedStyle.Render(": open workspace"))
	}
	parts = append(parts,
		shortcutKeyStyle.Render("s")+mutedStyle.Render(": sort"),
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

// Error details can contain project paths. Keep terminal controls and newlines
// in those paths from executing or breaking the single-line notice layout.
func safeNoticeText(message string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, ansi.Strip(message))
}

func clearNotice(revision int) tea.Cmd {
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg {
		return noticeExpiredMessage{revision: revision}
	})
}
