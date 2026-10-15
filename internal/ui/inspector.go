package ui

import (
	"strings"
	"time"

	"a-gent/internal/agent"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	inspectorColumnsMinWidth     = 64
	inspectorStackedActivityRows = 3
	inspectorThinkingMinRows     = 4
	inspectorDivider             = " │ "
)

// The inspector uses the same selected-session records for both panels. Activity
// must not hide the latest successful edit while an agent continues working.
func (model Model) inspectorView(width, height int) string {
	if height <= 0 {
		return ""
	}
	session, selected := model.selectedSession()
	if !selected {
		message := "No live sessions found."
		if len(model.sessions) > 0 {
			message = "No sessions match this filter."
		}
		if model.lastError != nil {
			message = "Could not read live sessions."
		}
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(mutedStyle.Render(message))
	}

	cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]

	identity := mainTextStyle.Bold(true).Render(safeDisplayText(sessionDisplayName(session))) + "  " +
		providerStyle(session.Provider).Render(safeDisplayText(session.Provider)) + mutedStyle.Render(" · ") +
		model.projectStyle(session.WorkingDirectory).Bold(false).Render(
			safeDisplayText(projectName(session.WorkingDirectory)),
		) +
		mutedStyle.Render("  "+displayDirectory(session.WorkingDirectory))
	header := alignedLine(identity, previewExpandShortcut(), width)
	if height == 1 {
		return header
	}

	contentHeight := height - 1
	var content string
	if width < inspectorColumnsMinWidth {
		// Stack summaries rather than squeezing two unreadable columns together.
		activityHeight := min(inspectorStackedActivityRows, max(1, contentHeight/2))
		activity := model.inspectorActivity(session, cached, width, activityHeight)
		edit := model.inspectorEdit(session, cached, width, contentHeight-activityHeight)
		content = activity + "\n" + edit
	} else {
		// Give the diff three fifths of the usable width.
		dividerWidth := lipgloss.Width(inspectorDivider)
		activityWidth := (width - dividerWidth) * 2 / 5
		editWidth := width - activityWidth - dividerWidth
		activity := model.inspectorActivity(session, cached, activityWidth, contentHeight)
		edit := model.inspectorEdit(session, cached, editWidth, contentHeight)
		divider := mutedStyle.Foreground(lipgloss.Color(colorDivider)).Render(
			strings.TrimSuffix(strings.Repeat(inspectorDivider+"\n", contentHeight), "\n"),
		)
		content = lipgloss.JoinHorizontal(lipgloss.Top, activity, divider, edit)
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(header + "\n" + content)
}

func (model Model) inspectorActivity(session agent.Session, cached previewEntry, width, height int) string {
	if height <= 0 {
		return ""
	}
	title := "ACTIVITY"
	if cached.err != nil || session.Stale || (!cached.checked.IsZero() && time.Since(cached.checked) > previewStaleAfter) {
		title += " · stale"
	}
	lines := []string{mutedStyle.Render(title)}
	if cached.activity == nil {
		message := model.previewStatus(session, true, cached)
		if cached.loaded && cached.err == nil && model.previewSources[session.Provider] != nil &&
			session.State == agent.StateIdle {
			message = "No current activity"
		}
		lines = append(lines, mutedStyle.Render(message))
	} else {
		activity := cached.activity
		label := safeDisplayText(activity.Label)
		if session.State == agent.StateWaiting {
			label = "Waiting for input · " + label
		} else if session.State == agent.StateIdle {
			label = "Last output · idle · " + label
		}
		lines = append(lines, statusStyle(sessionState(session)).Render(label))

		textLines := strings.Split(activity.Text, "\n")
		for index, line := range textLines {
			textLines[index] = safeDisplayText(strings.ReplaceAll(line, "\t", "  "))
		}
		text := strings.Join(textLines, "\n")
		lines = append(lines, strings.Split(ansi.Wrap(text, width, ""), "\n")...)
	}
	if model.previewThinking(session, cached) && height >= inspectorThinkingMinRows {
		lines = lines[:min(len(lines), height-1)]
		for len(lines) < height-1 {
			lines = append(lines, "")
		}
		lines = append(lines, model.thinkingIndicator(session.Provider, width))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(
		clipLines(strings.Join(lines, "\n"), width),
	)
}

func (model Model) inspectorEdit(session agent.Session, cached previewEntry, width, height int) string {
	if height <= 0 {
		return ""
	}
	if cached.edit == nil {
		message := model.previewStatus(session, true, cached)
		if cached.loaded && cached.err == nil && model.previewSources[session.Provider] != nil {
			message = "No edits yet"
		}
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(
			mutedStyle.Render("LATEST EDIT") + "\n" + mutedStyle.Render(message),
		)
	}
	return model.editPreviewView(session, cached, width, height, false, true)
}
