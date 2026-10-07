package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func (model Model) sessionTableView() string {
	columns := model.table.Columns()
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
		headerCells[index] = renderTableCell(title, column.Width, style, lipgloss.Color(colorSection))
	}

	rows := []string{renderSelectionCursor(false, lipgloss.Color(colorSection)) + lipgloss.JoinHorizontal(lipgloss.Top, headerCells...), ""}
	start, end := model.visibleSessionRange()
	for index := start; index < end; index++ {
		rows = append(rows, model.sessionRowView(index, columns))
	}

	for placeholderIndex := end - start; placeholderIndex < model.table.Height(); placeholderIndex++ {
		rows = append(rows, model.emptySessionRowView(columns, placeholderIndex == 0))
	}

	if len(model.filteredSessions()) > model.table.Height() {
		track := model.table.Height()
		thumbSize := max(1, track*track/len(model.filteredSessions()))
		thumbStart := start * (track - thumbSize) / (len(model.filteredSessions()) - track)
		for index := 0; index < track; index++ {
			glyph, style := "│", mutedStyle
			if index >= thumbStart && index < thumbStart+thumbSize {
				glyph, style = "┃", accentStyle
			}
			row := rows[index+2]
			rows[index+2] = ansi.Truncate(row, lipgloss.Width(row)-1, "") + style.Render(glyph)
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

	end := min(start+visibleRows, len(model.filteredSessions()))
	return start, end
}

func (model Model) sessionRowView(index int, columns []table.Column) string {
	selected := index == model.table.Cursor()
	background := lipgloss.Color("")
	if selected {
		background = lipgloss.Color(colorSelection)
	}

	cells := make([]string, len(columns))
	session := model.filteredSessions()[index]
	for columnIndex, column := range columns {
		value, style := sessionColumnValue(session, column.Title)
		if column.Title == "Session" {
			cells[columnIndex] = renderTableCell(session.Name, column.Width, mainTextStyle.Bold(selected), background)
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
		return session.Provider, providerStyle(session.Provider)
	case "Session":
		return session.Name, mainTextStyle
	case "Directory":
		return filepath.Base(session.WorkingDirectory), mutedStyle
	case "Status":
		return "● " + displayState(sessionState(session)), statusStyle(sessionState(session))
	case "Last active":
		// Use the provider timestamp when available; an active session without
		// a timestamp still has a meaningful fallback.
		if sessionState(session) == agent.StateRunning && session.LastActiveAt.IsZero() {
			return "Now", mutedStyle
		}
		return strings.TrimSuffix(formatLastActiveAt(session.LastActiveAt, time.Now()), " ago"), mutedStyle
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
	if model.orderHeld {
		model.rememberSessionOrder()
	}
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
