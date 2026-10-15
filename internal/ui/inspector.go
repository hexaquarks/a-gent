package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"a-gent/internal/agent"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	inspectorColumnsMinWidth       = 64
	inspectorHeaderSpacingMinRows  = 7
	inspectorStackedActivityRows   = 3
	inspectorStackedEditMinRows    = 2
	inspectorLabelBreathingRows    = 8
	inspectorWorkerSummaryMinRows  = 5
	inspectorDiffAddedBackground   = "#193A26"
	inspectorDiffRemovedBackground = "#3B2020"
	inspectorDivider               = " │ "
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

	header := model.inspectorIdentity(session, width)
	if height == 1 {
		return header
	}

	contentHeight := height - 1
	headerSpacing := "\n"
	if height >= inspectorHeaderSpacingMinRows {
		contentHeight--
		headerSpacing = "\n\n"
	}
	var content string
	if width < inspectorColumnsMinWidth {
		// Stack summaries rather than squeezing two unreadable columns together.
		activityHeight := min(inspectorStackedActivityRows, max(1, contentHeight-inspectorStackedEditMinRows))
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
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(header + headerSpacing + content)
}

// Each span carries the bar background because nested foreground styles reset
// terminal colors; styling only the outer line leaves gaps between identities.
func (model Model) inspectorIdentity(session agent.Session, width int) string {
	background := lipgloss.Color(colorSection)
	bar := lipgloss.NewStyle().Background(background)
	secondary := mutedStyle.Background(background)
	project := model.projectStyle(session.WorkingDirectory).Bold(true).Background(background).Render(
		safeDisplayText(projectName(session.WorkingDirectory)),
	)
	identity := mainTextStyle.Bold(true).Background(background).Render(safeDisplayText(sessionDisplayName(session))) +
		bar.Render("  ") + providerStyle(session.Provider).Bold(true).Background(background).Render(
		safeDisplayText(session.Provider),
	) + secondary.Render(" · ") + project + secondary.Render("  "+displayDirectory(session.WorkingDirectory))
	shortcut := shortcutKeyStyle.Background(background).Render("v") + secondary.Render(" expand")
	identity = clipLines(identity, max(1, width-lipgloss.Width(shortcut)-1))
	gap := bar.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(identity)-lipgloss.Width(shortcut))))
	return clipLines(identity+gap+shortcut, width)
}

func (model Model) inspectorActivity(session agent.Session, cached previewEntry, width, height int) string {
	if height <= 0 {
		return ""
	}
	title := "ACTIVITY"
	if cached.err != nil || session.Stale || (!cached.checked.IsZero() && time.Since(cached.checked) > previewStaleAfter) {
		title += " · stale"
	}
	lines := []string{inspectorLabel(title)}
	if height >= inspectorLabelBreathingRows {
		lines = append(lines, "")
	}
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
		lines = append(lines, statusStyle(sessionState(session)).Bold(true).Render(indicatorDot+" "+label))

		textLines := strings.Split(activity.Text, "\n")
		for index, line := range textLines {
			textLines[index] = safeDisplayText(strings.ReplaceAll(line, "\t", "  "))
		}
		text := strings.Join(textLines, "\n")
		lines = append(lines, strings.Split(ansi.Wrap(text, width, ""), "\n")...)
	}
	workers, running, _ := model.boardWorkerCounts(session)
	if workers > 0 && height >= inspectorWorkerSummaryMinRows {
		lines = lines[:min(len(lines), height-1)]
		for len(lines) < height-1 {
			lines = append(lines, "")
		}
		label := "subagents"
		if workers == 1 {
			label = "subagent"
		}
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("%d %s · %d running", workers, label, running)))
	}

	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(
		clipLines(strings.Join(lines, "\n"), width),
	)
}

func inspectorLabel(text string) string {
	return mutedStyle.Background(lipgloss.Color(colorSection)).Padding(0, 1).Render(text)
}

func (model Model) inspectorEdit(session agent.Session, cached previewEntry, width, height int) string {
	if height <= 0 {
		return ""
	}
	heading := inspectorLabel("LATEST EDIT")
	if cached.edit == nil {
		message := model.previewStatus(session, true, cached)
		if cached.loaded && cached.err == nil && model.previewSources[session.Provider] != nil {
			message = "No edits yet"
		}
		spacing := "\n"
		if height >= inspectorLabelBreathingRows {
			spacing = "\n\n"
		}
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(
			heading + spacing + mutedStyle.Render(message),
		)
	}
	edit := cached.edit
	added, removed := diffLineCounts(edit.Diff)
	counts := runningStyle.Render(fmt.Sprintf("+%d", added)) + " " + errorStyle.Render(fmt.Sprintf("−%d", removed))
	status := ""
	if cached.err != nil || session.Stale || time.Since(cached.checked) > previewStaleAfter {
		status += " · stale"
	}
	if edit.Truncated {
		status += " · truncated"
	}
	heading = inspectorLabel("LATEST EDIT" + status)
	filename := mutedStyle.Render(" " + safeDisplayText(edit.Filename))
	lines := []string{alignedLine(heading+filename, counts, width)}
	if height >= inspectorLabelBreathingRows {
		lines = append(lines, "")
	}
	rows := inspectorDiffRows(edit.Diff)
	if strings.TrimSpace(edit.Diff) == "" {
		rows = []inspectorDiffRow{{text: "Preview unavailable"}}
	}
	// Start near the first edit rather than spending the small panel on context.
	for index, row := range rows {
		if strings.HasPrefix(row.text, "+") || strings.HasPrefix(row.text, "-") {
			rows = rows[max(0, index-1):]
			break
		}
	}
	numberWidth := 1
	for _, row := range rows {
		if row.numberKnown {
			numberWidth = max(numberWidth, len(strconv.Itoa(row.number)))
		}
	}
	for _, row := range rows[:min(len(rows), max(0, height-len(lines)))] {
		number := ""
		if row.numberKnown {
			number = strconv.Itoa(row.number)
		}
		gutter := mutedStyle.Render(fmt.Sprintf("%*s ", numberWidth, number))
		contentWidth := max(1, width-numberWidth-1)
		style := mainTextStyle
		if strings.HasPrefix(row.text, "+") {
			style = runningStyle.Background(lipgloss.Color(inspectorDiffAddedBackground))
		} else if strings.HasPrefix(row.text, "-") {
			style = errorStyle.Background(lipgloss.Color(inspectorDiffRemovedBackground))
		}
		text := safeDisplayText(strings.ReplaceAll(row.text, "\t", "  "))
		lines = append(lines, gutter+style.Width(contentWidth).MaxWidth(contentWidth).Render(
			ansi.Truncate(text, contentWidth, "…"),
		))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

type inspectorDiffRow struct {
	number      int
	numberKnown bool
	text        string
}

// Line numbers come from provider-recorded hunk headers. Raw edit content can
// lack that metadata, so its gutter stays blank rather than inventing locations.
func inspectorDiffRows(diff string) []inspectorDiffRow {
	var rows []inspectorDiffRow
	oldLine, newLine := 0, 0
	numberKnown, inHunk := false, false
	for _, line := range strings.Split(strings.TrimSuffix(diff, "\n"), "\n") {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			fields := strings.Fields(line)
			numberKnown = false
			if len(fields) >= 4 && fields[3] == "@@" && strings.HasPrefix(fields[1], "-") && strings.HasPrefix(fields[2], "+") {
				oldStart := strings.SplitN(strings.TrimPrefix(fields[1], "-"), ",", 2)[0]
				newStart := strings.SplitN(strings.TrimPrefix(fields[2], "+"), ",", 2)[0]
				var oldErr, newErr error
				oldLine, oldErr = strconv.Atoi(oldStart)
				newLine, newErr = strconv.Atoi(newStart)
				numberKnown = oldErr == nil && newErr == nil && oldLine >= 0 && newLine >= 0
			}
			continue
		}
		if !inHunk && (strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")) {
			continue
		}
		if strings.HasPrefix(line, `\ No newline`) {
			continue
		}
		row := inspectorDiffRow{text: line, numberKnown: numberKnown}
		switch {
		case strings.HasPrefix(line, "-"):
			row.number = oldLine
			oldLine++
		case strings.HasPrefix(line, "+"):
			row.number = newLine
			newLine++
		case strings.HasPrefix(line, " "):
			row.number = newLine
			oldLine++
			newLine++
		default:
			row.numberKnown = false
		}
		rows = append(rows, row)
	}
	return rows
}
