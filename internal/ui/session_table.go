package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func (model Model) sessionTableView() string {
	columns := model.table.Columns()
	sessions := model.filteredSessions()
	headerCells := make([]string, len(columns))
	for index, column := range columns {
		title := column.Title
		if title == model.sort.column {
			// Reserve the arrow before truncating narrow column titles.
			title = runewidth.Truncate(title, max(0, column.Width-4), "…") + " " + model.sort.arrow()
		}
		style := sectionStyle
		if column.Title == "Last active" {
			style = style.Align(lipgloss.Right)
		}
		headerCells[index] = renderTableCell(title, column.Width, style, lipgloss.Color(colorTableHeader))
	}

	rows := []string{renderSelectionCursor(false, lipgloss.Color(colorTableHeader)) + lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)}
	start, end := model.visibleSessionRange()
	for index := start; index < end; index++ {
		rows = append(rows, model.sessionRowView(sessions[index], index == model.table.Cursor(), columns))
	}

	for placeholderIndex := end - start; placeholderIndex < model.table.Height(); placeholderIndex++ {
		rows = append(rows, model.emptySessionRowView(columns, placeholderIndex == 0))
	}

	// Keep the rail outside the cells and selection background. Reserving its
	// gutter for short lists too prevents columns moving when sessions arrive.
	for index := range rows {
		rows[index] += strings.Repeat(" ", scrollbarGutterWidth)
	}
	if len(sessions) > model.table.Height() {
		track := model.table.Height()
		thumbSize := max(1, track*track/len(sessions))
		thumbStart := start * (track - thumbSize) / (len(sessions) - track)
		for index := 0; index < track; index++ {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorDivider)).Background(lipgloss.Color(colorBackground))
			if index >= thumbStart && index < thumbStart+thumbSize {
				style = style.Foreground(lipgloss.Color(colorAccent))
			}
			row := rows[index+1]
			rows[index+1] = strings.TrimSuffix(row, " ") + style.Render("┃")
		}
	}
	return strings.Join(rows, "\n")
}

func (model Model) sessionTitle() string {
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return "SESSIONS"
	}

	start, end := model.visibleSessionRange()
	return fmt.Sprintf("SESSIONS (%d of %d)", end-start, len(sessions))
}

func (model Model) visibleSessionRange() (int, int) {
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return 0, 0
	}

	visibleRows := min(model.table.Height(), len(sessions))
	selectedIndex := model.table.Cursor()
	start := 0
	if selectedIndex >= visibleRows {
		start = selectedIndex - visibleRows + 1
	}

	end := min(start+visibleRows, len(sessions))
	return start, end
}

func (model Model) sessionRowView(session agent.Session, selected bool, columns []table.Column) string {
	background := lipgloss.Color("")
	if selected {
		background = lipgloss.Color(colorSelection)
	}
	background = model.readyPulseBackground(session, background)

	cells := make([]string, len(columns))
	for columnIndex, column := range columns {
		value, style := sessionColumnValue(session, column.Title)
		if column.Title == "Project" {
			style = model.projectStyle(session.WorkingDirectory)
		}
		if column.Title == "Session" {
			cells[columnIndex] = renderTableCell(sessionDisplayName(session), column.Width, mainTextStyle.Bold(selected), background)
			continue
		}
		if column.Title == "Last active" {
			style = style.Align(lipgloss.Right)
		}
		cells[columnIndex] = renderTableCell(value, column.Width, style, background)
	}

	return model.sessionGutter(session, selected, background) + lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

// The cursor and unread dot have separate cells, before the agent column.
func (model Model) sessionGutter(session agent.Session, selected bool, background lipgloss.Color) string {
	cursor, dot := " ", " "
	if selected {
		cursor = "›"
	}
	if model.unreadSessions[sessionIdentity{provider: session.Provider, id: session.ID}] {
		dot = "●"
	}
	style := accentStyle
	if background != "" {
		style = style.Background(background)
	}
	return style.Render(cursor + " " + dot + " ")
}

func (model Model) emptySessionRowView(columns []table.Column, showEmptyMessage bool) string {
	cells := make([]string, len(columns))
	emptyMessageColumn := 0
	emptyMessage := "No live"
	for columnIndex, column := range columns {
		if column.Title == "Session" {
			emptyMessageColumn = columnIndex
			emptyMessage = model.emptySessionMessage()
			break
		}
	}

	for columnIndex, column := range columns {
		value := ""
		style := lipgloss.NewStyle()
		if showEmptyMessage && columnIndex == emptyMessageColumn {
			value = emptyMessage
			style = mutedStyle
			// Empty rows can use the remaining columns to keep the message readable.
			for _, remainingColumn := range columns[columnIndex+1:] {
				column.Width += remainingColumn.Width
			}
			cells[columnIndex] = renderTableCell(value, column.Width, style, lipgloss.Color(""))
			cells = cells[:columnIndex+1]
			break
		}

		cells[columnIndex] = renderTableCell(value, column.Width, style, lipgloss.Color(""))
	}

	return renderSelectionCursor(false, lipgloss.Color("")) + lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func renderSelectionCursor(selected bool, background lipgloss.Color) string {
	value := "    "
	if selected {
		value = "›   "
	}
	style := accentStyle.Width(selectionCursorWidth)
	if background != "" {
		style = style.Background(background)
	}
	return style.Render(value)
}

func (model Model) emptySessionMessage() string {
	if len(model.sessions) == 0 {
		return "No live sessions found."
	}

	return "No sessions match this filter."
}

func sessionColumnValue(session agent.Session, columnTitle string) (string, lipgloss.Style) {
	switch columnTitle {
	case "Agent":
		return session.Provider, providerStyle(session.Provider).Bold(true)
	case "Session":
		return sessionDisplayName(session), mainTextStyle
	case "Project":
		return projectName(session.WorkingDirectory), mutedStyle
	case "Directory":
		return filepath.Base(session.WorkingDirectory), mutedStyle
	case "Status":
		return "● " + displayState(sessionState(session)), statusStyle(sessionState(session))
	case "Last active":
		// Use the provider timestamp when available; an active session without
		// a timestamp still has a meaningful fallback.
		if sessionState(session) == agent.StateRunning && session.LastActiveAt.IsZero() {
			return "Now", runningStyle
		}
		now := time.Now()
		return strings.TrimSuffix(formatLastActiveAt(session.LastActiveAt, now), " ago"), lastActiveStyle(session.LastActiveAt, now)
	default:
		return "", lipgloss.NewStyle()
	}
}

func renderTableCell(value string, width int, textStyle lipgloss.Style, background lipgloss.Color) string {
	contentWidth := max(0, width-2)
	truncatedValue := runewidth.Truncate(safeDisplayText(value), contentWidth, "…")
	cellStyle := textStyle.Width(width).MaxWidth(width).Padding(0, 1)
	if background != "" {
		cellStyle = cellStyle.Background(background)
	}

	return cellStyle.Render(truncatedValue)
}

func (model *Model) updateTableRows() {
	// Read identity from the previous rows; the provider data or sort order
	// may already have changed, so the old cursor no longer identifies it.
	var selectedID sessionIdentity
	hasSelection := model.table.Cursor() >= 0 && model.table.Cursor() < len(model.tableSessionIDs)
	if hasSelection {
		selectedID = model.tableSessionIDs[model.table.Cursor()]
	}

	columns := model.table.Columns()
	sessions := model.filteredSessions()
	rows := make([]table.Row, len(sessions))
	model.tableSessionIDs = make([]sessionIdentity, len(sessions))
	for index, session := range sessions {
		model.tableSessionIDs[index] = sessionIdentity{provider: session.Provider, id: session.ID}
		values := make([]string, len(columns))
		for columnIndex, column := range columns {
			values[columnIndex], _ = sessionColumnValue(session, column.Title)
		}
		rows[index] = table.Row(values)
	}

	model.table.SetRows(rows)
	if len(rows) == 0 {
		model.table.SetCursor(0)
		return
	}

	model.table.SetCursor(max(0, min(model.table.Cursor(), len(rows)-1)))
	if hasSelection {
		for index, id := range model.tableSessionIDs {
			if id == selectedID {
				model.table.SetCursor(index)
				break
			}
		}
	}
}

// sessionDisplayName supplies a label when a provider has not named a session.
func sessionDisplayName(session agent.Session) string {
	if strings.TrimSpace(safeDisplayText(session.Name)) == "" {
		return "Untitled session"
	}
	return session.Name
}
