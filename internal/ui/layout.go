package ui

import "github.com/charmbracelet/bubbles/table"

const (
	defaultTableWidth   = 70
	minimumTableWidth   = 34
	sidebarContentWidth = 22
	minimumSessionRows  = 3
	maximumSessionRows  = 8

	// popupChromeRows covers the header, table title and header, selected-session
	// panel, and footer around the reserved session rows.
	popupChromeRows = 15
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
		tableWidth = model.width - sidebarContentWidth - 6
	}
	tableWidth = max(tableWidth, minimumTableWidth)

	columns := tableColumns(tableWidth)
	model.table.SetWidth(tableWidth)
	model.table.SetColumns(columns)
	model.updateTableRows()

	tableHeight := maximumSessionRows
	if model.height > 0 {
		tableHeight = min(tableHeight, max(minimumSessionRows, model.height-10))
	}
	model.table.SetHeight(tableHeight + 1)
}

func tableColumns(tableWidth int) []table.Column {
	if tableWidth < 48 {
		return []table.Column{{Title: "Agent", Width: 12}, {Title: "Status", Width: tableWidth - 15}}
	}
	if tableWidth < 72 {
		return []table.Column{
			{Title: "Agent", Width: 12},
			{Title: "Session", Width: tableWidth - 29},
			{Title: "Status", Width: 12},
		}
	}

	return []table.Column{
		{Title: "Agent", Width: 12},
		{Title: "Session", Width: tableWidth / 3},
		{Title: "Directory", Width: tableWidth/3 - 3},
		{Title: "Status", Width: 12},
	}
}
