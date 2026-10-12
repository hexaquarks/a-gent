package ui

import (
	"fmt"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

func (model Model) boardFocusView(width int) string {
	width = max(1, width)
	height := model.boardFocusHeight()
	session, selected := model.selectedSession()
	shortcut := shortcutKeyStyle.Render("v") + mutedStyle.Render(" details / diff")
	if width < 32 {
		shortcut = shortcutKeyStyle.Render("v") + mutedStyle.Render(" expand")
	}
	if !selected {
		message := "No sessions match this filter."
		if model.lastError != nil {
			message = "Could not read live sessions."
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(clipLines(
			accentStyle.Render("FOCUS ")+mutedStyle.Render(message), width,
		))
	}

	title := sectionBar("FOCUS", 7, model.panelTitleStyle()) + " " +
		mainTextStyle.Bold(true).Render(safeDisplayText(sessionDisplayName(session)))
	metadata := providerStyle(session.Provider).Render(safeDisplayText(session.Provider)) + mutedStyle.Render(" · ") +
		model.projectStyle(session.WorkingDirectory).Render(safeDisplayText(projectName(session.WorkingDirectory))) +
		mutedStyle.Render(" · ") + statusStyle(sessionState(session)).Render(displayState(sessionState(session)))
	summary, status := model.boardPreviewSummary(session)

	var content string
	if height == 1 {
		content = alignedLine(title, shortcut, width)
	} else if width < 72 {
		previewActions := shortcut
		statusWidth := width - lipgloss.Width(shortcut) - 2
		if status != "" && statusWidth > 0 {
			previewActions = clipLines(status, statusWidth) + " " + shortcut
		}
		content = clipLines(title, width) + "\n" + clipLines(metadata, width)
		if height > 2 {
			content += "\n" + alignedLine(summary, previewActions, width)
		} else {
			content = clipLines(title, width) + "\n" + alignedLine(metadata, shortcut, width)
		}
	} else {
		focusWidth := width * 2 / 5
		previewWidth := width - focusWidth - 3
		focus := lipgloss.NewStyle().Width(focusWidth).Render(clipLines(title+"\n"+metadata, focusWidth))
		preview := lipgloss.NewStyle().Width(previewWidth).Render(
			clipLines(summary, previewWidth) + "\n" + alignedLine(status, shortcut, previewWidth),
		)
		divider := mutedStyle.Foreground(lipgloss.Color(colorDivider)).Render(" │ \n │ ")
		content = lipgloss.JoinHorizontal(lipgloss.Top, focus, divider, preview)
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(content)
}

// Summaries use only the selected session's cached records, with the same
// activity preference and stale-data signals as the expanded preview.
func (model Model) boardPreviewSummary(session agent.Session) (string, string) {
	cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]
	stale := cached.err != nil || session.Stale || time.Since(cached.checked) > 5*time.Second
	status := ""
	if model.showActivity(session, cached) {
		summary := "Activity: " + safeDisplayText(cached.activity.Label) + " · " + safeDisplayText(cached.activity.Text)
		if stale {
			status = waitingStyle.Render("stale")
		}
		return mutedStyle.Render(summary), status
	}
	if cached.edit != nil {
		added, removed := diffLineCounts(cached.edit.Diff)
		status = runningStyle.Render(fmt.Sprintf("+%d", added)) + " " + errorStyle.Render(fmt.Sprintf("−%d", removed))
		if cached.edit.Truncated {
			status += mutedStyle.Render(" · truncated")
		}
		if stale {
			status = waitingStyle.Render("stale · ") + status
		}
		return mutedStyle.Render("Last edit: " + safeDisplayText(cached.edit.Filename)), status
	}
	return mutedStyle.Render(model.previewStatus(session, true, cached)), status
}
