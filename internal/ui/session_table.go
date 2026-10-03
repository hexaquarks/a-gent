package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func (model Model) sessionTableView() string {
	columns := model.table.Columns()
	headerCells := make([]string, len(columns))
	for index, column := range columns {
		headerCells[index] = renderTableCell(column.Title, column.Width, sectionStyle, lipgloss.Color(colorDivider))
	}

	rows := []string{lipgloss.JoinHorizontal(lipgloss.Top, headerCells...), ""}
	start, end := model.visibleSessionRange()
	for index := start; index < end; index++ {
		rows = append(rows, model.sessionRowView(index, columns))
	}

	for placeholderIndex := end - start; placeholderIndex < model.table.Height(); placeholderIndex++ {
		rows = append(rows, model.emptySessionRowView(columns, placeholderIndex == 0))
	}

	return strings.Join(rows, "\n")
}

func (model Model) sessionTitle() string {
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return "SESSIONS"
	}

	start, end := model.visibleSessionRange()
	return fmt.Sprintf("SESSIONS (%d-%d of %d)", start+1, end, len(sessions))
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
	for columnIndex, column := range columns {
		session := model.filteredSessions()[index]
		value, style := sessionColumnValue(session, column.Title)
		if selected && column.Title == "Agent" {
			value = "› " + value
		}
		cells[columnIndex] = renderTableCell(value, column.Width, style, background)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
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

	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
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
		return session.Provider, agentStyle
	case "Session":
		return session.Name, mainTextStyle
	case "Directory":
		return filepath.Base(session.WorkingDirectory), mutedStyle
	case "Status":
		return "● " + displayState(session.State), statusStyle(session.State)
	default:
		return "", lipgloss.NewStyle()
	}
}

func renderTableCell(value string, width int, textStyle lipgloss.Style, background lipgloss.Color) string {
	contentWidth := max(0, width-2)
	truncatedValue := runewidth.Truncate(value, contentWidth, "…")
	cellStyle := textStyle.Width(width).MaxWidth(width).Padding(0, 1)
	if background != "" {
		cellStyle = cellStyle.Background(background)
	}

	return cellStyle.Render(truncatedValue)
}

func (model *Model) updateTableRows() {
	columns := model.table.Columns()
	sessions := model.filteredSessions()
	rows := make([]table.Row, len(sessions))
	for index, session := range sessions {
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
	model.table.SetCursor(min(model.table.Cursor(), len(rows)-1))
}
