package ui

import (
	"fmt"
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

type boardColumn struct {
	title   string
	color   string
	indexes []int
}

func boardColumnIndex(session agent.Session) int {
	switch sessionState(session) {
	case agent.StateRunning:
		return 1
	case agent.StateIdle:
		return 2
	default:
		// Errors and unreadable states need attention rather than appearing idle.
		return 0
	}
}

func (model Model) boardColumns() []boardColumn {
	columns := []boardColumn{
		{title: "NEEDS YOU", color: colorAttention},
		{title: "RUNNING", color: colorRunning},
		{title: "IDLE", color: colorSecondary},
	}

	for index, session := range model.filteredSessions() {
		column := boardColumnIndex(session)
		columns[column].indexes = append(columns[column].indexes, index)
	}
	return columns
}

// Keep selection in the existing table so opening, filtering, and previews
// share one session identity across layouts. Board movement follows card order.
func (model *Model) moveBoardCursor(direction int) {
	var indexes []int
	for _, column := range model.boardColumns() {
		indexes = append(indexes, column.indexes...)
	}

	for position, index := range indexes {
		if index == model.table.Cursor() {
			position = max(0, min(len(indexes)-1, position+direction))
			model.table.SetCursor(indexes[position])
			return
		}
	}
}

func (model Model) boardView() string {
	width := max(1, model.table.Width()-panelStyle.GetHorizontalFrameSize())
	height := model.table.Height() + 1
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(
			mutedStyle.Render("No sessions match this filter."),
		)
	}

	columns := model.boardColumns()

	// A narrow terminal shows the selected status column at full width.
	// Moving through cards still reaches every column without tiny card labels.
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
	gap := strings.Repeat(" \n", height-1) + " "
	return lipgloss.JoinHorizontal(lipgloss.Top, views[0], gap, views[1], gap, views[2])
}

func (model Model) boardColumnView(column boardColumn, sessions []agent.Session, width, height int) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(column.color)).Bold(true)
	heading := fmt.Sprintf("%s · %d", column.title, len(column.indexes))
	rows := []string{style.Render(clipLines(heading, width))}

	capacity := max(1, height/4)
	start := 0
	for position, index := range column.indexes {
		if index == model.table.Cursor() {
			start = max(0, position-capacity+1)
			break
		}
	}

	for _, index := range column.indexes[start:min(len(column.indexes), start+capacity)] {
		rows = append(rows, model.boardCardView(sessions[index], index == model.table.Cursor(), width), "")
	}
	if len(column.indexes) == 0 {
		rows = append(rows, mutedStyle.Render("No sessions"))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n"))
}

func (model Model) boardCardView(session agent.Session, selected bool, width int) string {
	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	background := lipgloss.Color(colorBackground)
	if selected {
		background = lipgloss.Color(colorSelection)
	}
	cursorStyle := accentStyle.Background(background)
	cursor := "  "
	if selected {
		cursor = cursorStyle.Render("› ")
		if model.unreadSessions[identity] {
			cursor = cursorStyle.Render("›●")
		}
	} else if model.unreadSessions[identity] {
		cursor = cursorStyle.Render("● ")
	}

	contentWidth := max(1, width-2)
	title := mainTextStyle.Bold(true).Background(background).Render(
		clipLines(safeDisplayText(sessionDisplayName(session)), contentWidth),
	)
	provider := providerStyle(session.Provider).Background(background).Render(safeDisplayText(session.Provider))
	project := model.projectStyle(session.WorkingDirectory).Background(background).Render(
		safeDisplayText(projectName(session.WorkingDirectory)),
	)
	state := statusStyle(sessionState(session)).Background(background).Render(displayState(sessionState(session)))
	activity := mutedStyle.Background(background).Render(formatLastActiveAt(session.LastActiveAt, time.Now()))
	separator := mutedStyle.Background(background).Render(" · ")
	lines := []string{
		cursor + title,
		"  " + clipLines(provider+separator+project, contentWidth),
		"  " + clipLines(state+separator+activity, contentWidth),
	}
	style := lipgloss.NewStyle().Width(width).Background(background)
	return style.Render(strings.Join(lines, "\n"))
}
