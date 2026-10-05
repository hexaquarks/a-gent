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
		headerCells[index] = renderTableCell(title, column.Width, sectionStyle, lipgloss.Color(colorDivider))
	}

	rows := []string{renderSelectionCursor(false, lipgloss.Color(colorDivider)) + lipgloss.JoinHorizontal(lipgloss.Top, headerCells...), ""}
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
	session := model.filteredSessions()[index]
	for columnIndex, column := range columns {
		value, style := sessionColumnValue(session, column.Title)
		if column.Title == "Session" {
			cells[columnIndex] = model.sessionNameCell(session, column.Width, background)
			continue
		}
		cells[columnIndex] = renderTableCell(value, column.Width, style, background)
	}

	return renderSelectionCursor(selected, background) + lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func (model Model) sessionNameCell(session agent.Session, width int, background lipgloss.Color) string {
	// Always reserve the dot's space so names stay aligned when it is cleared.
	marker := "  "
	if model.unreadSessions[sessionIdentity{provider: session.Provider, id: session.ID}] {
		markerStyle := accentStyle
		if background != "" {
			markerStyle = markerStyle.Background(background)
		}
		marker = markerStyle.Render("● ")
	}
	name := runewidth.Truncate(safeDisplayText(session.Name), max(0, width-4), "…")
	style := mainTextStyle.Width(width).MaxWidth(width).Padding(0, 1)
	if background != "" {
		style = style.Background(background)
	}
	return style.Render(marker + name)
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
	value := " "
	if selected {
		value = "›"
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
		// Running sessions are active now even when their provider timestamp
		// advances between polls or has not been populated yet.
		if sessionState(session) == agent.StateRunning {
			return "Now", mutedStyle
		}
		return formatLastActiveAt(session.LastActiveAt, time.Now()), mutedStyle
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
