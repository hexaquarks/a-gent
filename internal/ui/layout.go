package ui

import "github.com/charmbracelet/bubbles/table"

const (
	selectionCursorWidth = 3
	defaultTableWidth    = 70
	minimumTableWidth    = 34
	sidebarContentWidth  = 24
	maximumSessionRows   = 8

	// popupChromeRows covers the header, table title and header, selected-session
	// panel, and footer around the reserved session rows.
	popupChromeRows = 19
	// popupBorderRows are drawn by tmux and are not available to the program.
	popupBorderRows = 2
	// PopupContentHeight is the terminal-row height needed by the dashboard.
	PopupContentHeight = maximumSessionRows + popupChromeRows
	// PopupHeight includes the tmux border so the dashboard header is never
	// clipped by the popup's interior height.
	PopupHeight = PopupContentHeight + popupBorderRows
)

func (model *Model) resizeTable() {
	tableWidth := defaultTableWidth
	if model.width > 0 {
		tableWidth = model.width - sidebarContentWidth - 3
	}
	if model.width > 0 && model.width < 64 {
		tableWidth = max(8, model.width-2)
	}
	tableWidth = max(tableWidth, 8)

	columns := tableColumns(tableWidth)
	model.table.SetWidth(tableWidth)
	// Bubbles renders immediately on SetColumns; old rows may have more cells.
	model.table.SetRows(nil)
	model.table.SetColumns(columns)

	visibleSortColumn := false
	for _, column := range columns {
		if column.Title == model.sort.column {
			visibleSortColumn = true
			break
		}
	}
	if !visibleSortColumn {
		// Keep the active sort indicator on screen when a column is hidden.
		model.sort = sessionSort{column: "Last active", descending: true}
	}
	model.updateTableRows()

	tableHeight := maximumSessionRows
	if model.height > 0 {
		tableHeight = max(1, min(maximumSessionRows, model.bodyHeight()-15))
		if model.table.Width() < 66 {
			tableHeight = max(1, min(maximumSessionRows, model.bodyHeight()-16))
		}
	}
	model.table.SetHeight(tableHeight + 1)
}

func tableColumns(tableWidth int) []table.Column {
	tableWidth -= panelStyle.GetHorizontalFrameSize() + selectionCursorWidth
	if tableWidth < 27 {
		return []table.Column{{Title: "Session", Width: max(1, tableWidth)}}
	}
	const lastActiveWidth = 16
	if tableWidth < 44 {
		return []table.Column{
			{Title: "Agent", Width: 8},
			{Title: "Status", Width: tableWidth - 8 - lastActiveWidth},
			{Title: "Last active", Width: lastActiveWidth},
		}
	}
	agentWidth, statusWidth := 12, 18
	if tableWidth < 66 {
		agentWidth, statusWidth = 8, 15
	}
	return []table.Column{
		{Title: "Agent", Width: agentWidth},
		{Title: "Session", Width: tableWidth - agentWidth - statusWidth - lastActiveWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Last active", Width: lastActiveWidth},
	}
}
