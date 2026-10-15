package ui

import (
	"fmt"
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	boardColumnHeadingRows = 2
	boardCardContentRows   = 3
	boardCardBorderRows    = 2
)

type boardColumn struct {
	title   string
	color   string
	indexes []int
}

func boardColumnIndex(session agent.Session) int {
	switch sessionState(session) {
	case agent.StateRunning:
		return 0
	case agent.StateIdle:
		return 1
	default:
		// Errors and unreadable states need attention rather than appearing idle.
		return 2
	}
}

func (model Model) boardColumns() []boardColumn {
	columns := []boardColumn{
		{title: "running", color: colorRunning},
		{title: "idle", color: colorSecondary},
		{title: "needs you", color: colorAttention},
	}

	for index, session := range model.filteredSessions() {
		column := boardColumnIndex(session)
		columns[column].indexes = append(columns[column].indexes, index)
	}
	return columns
}

// Keep selection in the table so opening, filtering, and previews share one
// session identity. Vertical movement stays within the selected status column.
func (model *Model) moveBoardCursor(direction int) {
	for _, column := range model.boardColumns() {
		for row, index := range column.indexes {
			if index == model.table.Cursor() {
				row = max(0, min(len(column.indexes)-1, row+direction))
				model.table.SetCursor(column.indexes[row])
				return
			}
		}
	}
}

// Horizontal movement skips empty columns and uses the same card row, clamped
// to the last card when the destination column is shorter. It never wraps.
func (model *Model) moveBoardColumn(direction int) {
	columns := model.boardColumns()
	for columnIndex, column := range columns {
		for row, index := range column.indexes {
			if index != model.table.Cursor() {
				continue
			}

			for next := columnIndex + direction; next >= 0 && next < len(columns); next += direction {
				indexes := columns[next].indexes
				if len(indexes) > 0 {
					model.table.SetCursor(indexes[min(row, len(indexes)-1)])
					return
				}
			}
			return
		}
	}
}

func (model Model) boardView() string {
	width := max(1, model.table.Width()-panelStyle.GetHorizontalFrameSize())
	height := model.boardHeight()
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(
			mutedStyle.Render("No sessions match this filter."),
		)
	}

	columns := model.boardColumns()

	// A narrow terminal shows the selected status column at full width.
	// Horizontal navigation changes the visible column without tiny card labels.
	if width < 72 {
		column := boardColumnIndex(sessions[model.table.Cursor()])
		return model.boardColumnView(columns[column], sessions, width, height)
	}

	columnWidth := (width - 2) / 3
	views := make([]string, len(columns))
	for index, column := range columns {
		currentWidth := columnWidth
		if index == len(columns)-1 {
			currentWidth = width - 2 - 2*columnWidth
		}
		views[index] = model.boardColumnView(column, sessions, currentWidth, height)
	}
	divider := mutedStyle.Foreground(lipgloss.Color(colorDivider)).Render("│")
	gap := strings.Repeat(divider+"\n", height-1) + divider
	return lipgloss.JoinHorizontal(lipgloss.Top, views[0], gap, views[1], gap, views[2])
}

func (model Model) boardColumnView(column boardColumn, sessions []agent.Session, width, height int) string {
	if height <= 2 && len(column.indexes) > 0 {
		index := column.indexes[0]
		for _, candidate := range column.indexes {
			if candidate == model.table.Cursor() {
				index = candidate
				break
			}
		}
		card := model.boardCardView(sessions[index], index == model.table.Cursor(), width)
		return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Split(card, "\n")[1])
	}

	// Worker summaries add a row to parent cards. Fit complete cards around
	// selection rather than assuming every card has the same height.
	selectedPosition := 0
	for position, index := range column.indexes {
		if index == model.table.Cursor() {
			selectedPosition = position
			break
		}
	}

	availableRows := max(0, height-boardColumnHeadingRows)
	start := 0
	usedRows := 0
	for position := 0; position <= selectedPosition && position < len(column.indexes); position++ {
		usedRows += model.boardCardHeight(sessions[column.indexes[position]]) - 1
	}
	for usedRows > availableRows && start < selectedPosition {
		usedRows -= model.boardCardHeight(sessions[column.indexes[start]]) - 1
		start++
	}

	end := start
	usedRows = 0
	for end < len(column.indexes) {
		cardRows := model.boardCardHeight(sessions[column.indexes[end]]) - 1
		if usedRows+cardRows > availableRows && end > start {
			break
		}
		usedRows += cardRows
		end++
		if usedRows >= availableRows {
			break
		}
	}

	style := lipgloss.NewStyle().Foreground(lipgloss.Color(column.color)).Bold(true)
	heading := fmt.Sprintf("%s · %d", column.title, len(column.indexes))
	more := ""
	if start > 0 {
		more += fmt.Sprintf(" ↑%d", start)
	}
	if end < len(column.indexes) {
		more += fmt.Sprintf(" ↓%d", len(column.indexes)-end)
	}
	heading = clipLines(heading, max(0, width-ansi.StringWidth(more))) + more
	rows := []string{
		style.Render(clipLines(heading, width)),
		style.Render(strings.Repeat("─", width)),
	}

	for _, index := range column.indexes[start:end] {
		card := model.boardCardView(sessions[index], index == model.table.Cursor(), width)
		if height < 7 {
			// Keep the title visible when the terminal cannot fit a full card frame.
			rows = append(rows, strings.Split(card, "\n")[1:4]...)
			continue
		}

		cardRows := strings.Split(card, "\n")
		// Keep every title at the same row, including when the first card is selected.
		if index == model.table.Cursor() {
			rows = rows[:len(rows)-1]
		} else {
			cardRows = cardRows[1:]
		}
		rows = append(rows, cardRows...)
	}
	if len(column.indexes) == 0 {
		rows = append(rows, mutedStyle.Render("No sessions"))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n"))
}

func (model Model) boardCardView(session agent.Session, selected bool, width int) string {
	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	background := lipgloss.Color(colorSection)
	if selected {
		background = lipgloss.Color(colorSelection)
	}
	marker := ""
	if model.unreadSessions[identity] {
		marker = accentStyle.Background(background).Render(indicatorDot + " ")
	}

	// Reserve the outer frame and the colored status bar.
	contentWidth := max(1, width-3)
	title := mainTextStyle.Bold(true).Background(background).Render(
		clipLines(safeDisplayText(sessionDisplayName(session)), contentWidth-lipgloss.Width(marker)),
	)
	provider := providerStyle(session.Provider).Background(background).Render(safeDisplayText(session.Provider))
	project := model.projectStyle(session.WorkingDirectory).Background(background).Render(
		safeDisplayText(projectName(session.WorkingDirectory)),
	)
	summaryStyle := mutedStyle.Background(background)
	if sessionState(session) == agent.StateWaiting || sessionState(session) == agent.StateError ||
		sessionState(session) == agent.StateUnavailable {
		summaryStyle = waitingStyle.Background(background)
	}
	summary := summaryStyle.Render(model.boardActivitySummary(session, time.Now()))
	activity := mutedStyle.Background(background).Render(formatLastActiveAt(session.LastActiveAt, time.Now()))
	separator := mutedStyle.Background(background).Render(" · ")

	// Short cards keep the provider/project readable; wider cards align the age
	// against the right edge without letting a long project push it out.
	metadata := provider + separator + project
	activityWidth := ansi.StringWidth(activity)
	if contentWidth >= activityWidth+8 {
		metadataWidth := contentWidth - activityWidth - 1
		metadata = lipgloss.NewStyle().Width(metadataWidth).Background(background).
			Render(clipLines(metadata, metadataWidth)) +
			lipgloss.NewStyle().Background(background).Render(" ") + activity
	} else {
		metadata = clipLines(metadata, contentWidth)
	}

	edge := statusStyle(sessionState(session)).Background(background).Render("▌")
	lineStyle := lipgloss.NewStyle().Width(contentWidth).Background(background)
	lines := []string{
		edge + lineStyle.Render(marker+title),
		edge + lineStyle.Render(metadata),
		edge + lineStyle.Render(clipLines(summary, contentWidth)),
	}
	workers, running := model.boardWorkerCounts(session)
	if workers > 0 {
		workerSummary := mutedStyle.Background(background).Render(fmt.Sprintf("%d subagents · ", workers)) +
			runningStyle.Background(background).Render(fmt.Sprintf("%d running", running))
		lines = append(lines, edge+lineStyle.Render(clipLines(workerSummary, contentWidth)))
	}

	border := lipgloss.NormalBorder()
	borderColor := lipgloss.Color(colorBackground)
	if selected {
		borderColor = lipgloss.Color(colorAccent)
	}
	return lipgloss.NewStyle().Background(background).
		Border(border).BorderForeground(borderColor).BorderBackground(lipgloss.Color(colorBackground)).
		Render(strings.Join(lines, "\n"))
}

func (model Model) boardWorkerCounts(session agent.Session) (total, running int) {
	if session.ID == "" {
		return 0, 0
	}

	for _, candidate := range model.sessions {
		if candidate.Provider != session.Provider || candidate.ParentID != session.ID {
			continue
		}
		total++
		if sessionState(candidate) == agent.StateRunning {
			running++
		}
	}
	return total, running
}

func (model Model) boardCardHeight(session agent.Session) int {
	height := boardCardContentRows + boardCardBorderRows
	if total, _ := model.boardWorkerCounts(session); total > 0 {
		height++
	}
	return height
}
