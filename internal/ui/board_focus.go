package ui

import (
	"fmt"
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

const (
	boardFocusColumnsMinWidth    = 72
	boardFocusRoomyMinHeight     = 32
	boardFocusShortShortcutWidth = 32
)

func (model Model) boardFocusView(width int) string {
	width = max(1, width)
	height := model.boardFocusHeight()
	session, selected := model.selectedSession()
	shortcut := shortcutKeyStyle.Render("v") + mutedStyle.Render(" details / diff")
	if width < boardFocusShortShortcutWidth {
		shortcut = previewExpandShortcut()
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
	project := model.projectStyle(session.WorkingDirectory).Render(
		safeDisplayText(projectName(session.WorkingDirectory)),
	)
	identity := providerStyle(session.Provider).Render(safeDisplayText(session.Provider)) +
		mutedStyle.Render(" · ") + project
	activity := model.boardFocusActivity(session)
	edit, status := model.boardPreviewSummary(session)

	var content string
	switch {
	case height == 1:
		content = alignedLine(title, shortcut, width)
	case width < boardFocusColumnsMinWidth:
		if height > 2 {
			content = alignedLine(title, shortcut, width) + "\n" + clipLines(activity, width) + "\n" +
				alignedLine(edit, status, width)
		} else {
			content = clipLines(title, width) + "\n" + alignedLine(activity, shortcut, width)
		}
	default:
		// Most of the strip belongs to the task and activity. The latest edit needs
		// only a filename and counts because v opens the full diff.
		dividerWidth := lipgloss.Width(inspectorDivider)
		focusWidth := (width - dividerWidth) * 3 / 5
		previewWidth := width - focusWidth - dividerWidth
		focusLines := []string{title, activity}
		previewLines := []string{
			clipLines(edit, previewWidth),
			alignedLine(status, shortcut, previewWidth),
		}
		if height > 2 {
			focusLines = []string{title, identity, activity}
			previewLines = []string{
				alignedLine(edit, status, previewWidth),
				"",
				alignedLine("", shortcut, previewWidth),
			}
		} else if lipgloss.Width(title)+lipgloss.Width(identity)+2 <= focusWidth {
			focusLines[0] = title + "  " + identity
		}

		focus := lipgloss.NewStyle().Width(focusWidth).Render(clipLines(strings.Join(focusLines, "\n"), focusWidth))
		preview := lipgloss.NewStyle().Width(previewWidth).Render(strings.Join(previewLines, "\n"))
		divider := mutedStyle.Foreground(lipgloss.Color(colorDivider)).Render(
			strings.TrimSuffix(strings.Repeat(inspectorDivider+"\n", height), "\n"),
		)
		content = lipgloss.JoinHorizontal(lipgloss.Top, focus, divider, preview)
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(content)
}

// The compact inspector keeps public output and the latest successful edit
// separate, so continuing work cannot hide a completed file change.
func (model Model) boardFocusActivity(session agent.Session) string {
	cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]
	if cached.activity == nil {
		message := model.previewStatus(session, true, cached)
		if cached.loaded && cached.err == nil && model.previewSources[session.Provider] != nil &&
			session.State == agent.StateIdle {
			message = "No current activity"
		}
		return mutedStyle.Render(message)
	}

	activity := safeDisplayText(cached.activity.Label) + " · " + safeDisplayText(cached.activity.Text)
	if cached.err != nil || session.Stale || time.Since(cached.checked) > previewStaleAfter {
		return waitingStyle.Render("stale · ") + mutedStyle.Render(activity)
	}
	return mutedStyle.Render(activity)
}

func (model Model) boardPreviewSummary(session agent.Session) (string, string) {
	cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]
	if cached.edit == nil {
		message := model.previewStatus(session, true, cached)
		if cached.loaded && cached.err == nil && model.previewSources[session.Provider] != nil {
			message = "No edits yet"
		}
		return mutedStyle.Render(message), ""
	}

	added, removed := diffLineCounts(cached.edit.Diff)
	status := runningStyle.Render(fmt.Sprintf("+%d", added)) + " " + errorStyle.Render(fmt.Sprintf("−%d", removed))
	if cached.edit.Truncated {
		status += mutedStyle.Render(" · truncated")
	}
	if cached.err != nil || session.Stale || time.Since(cached.checked) > previewStaleAfter {
		status = waitingStyle.Render("stale · ") + status
	}
	return mutedStyle.Render("Last edit: " + safeDisplayText(cached.edit.Filename)), status
}
