package ui

import (
	"a-gent/internal/agent"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (model Model) previewStatus(session agent.Session, selected bool, cached previewEntry) string {
	switch {
	case !selected:
		return "Select a session"
	case model.previewSources[session.Provider] == nil:
		return "Preview not supported"
	case cached.err != nil:
		return "Preview unavailable"
	case !cached.loaded:
		return "Loading preview…"
	case session.State == agent.StateRunning || session.State == agent.StateWaiting:
		return "Waiting for output"
	default:
		return "No edits yet"
	}
}

func (model Model) emptyPreviewView(message string, width, height int, expanded bool) string {
	text := mutedStyle.Render(ansi.Wrap(message, width, ""))
	if !expanded {
		return lipgloss.NewStyle().MaxHeight(height).Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text))
	}
	title := accentStyle.Render(ansi.Truncate("PREVIEW · Esc/q: return", width, "…"))
	return lipgloss.NewStyle().Width(width).MaxHeight(height).Render(title + "\n" + lipgloss.Place(width, max(0, height-1), lipgloss.Center, lipgloss.Center, text))
}
