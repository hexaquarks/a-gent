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
	const lastActiveWidth = 16

	// Leave room for the surrounding panel's padding so rows do not wrap.
	tableWidth -= panelStyle.GetHorizontalFrameSize()

	if tableWidth < 60 {
		return []table.Column{
			{Title: "Agent", Width: 8},
			{Title: "Status", Width: tableWidth - 8 - lastActiveWidth},
			{Title: "Last active at", Width: lastActiveWidth},
		}
	}
	if tableWidth < 90 {
		return []table.Column{
			{Title: "Agent", Width: 12},
			{Title: "Session", Width: tableWidth - 24 - lastActiveWidth},
			{Title: "Status", Width: 12},
			{Title: "Last active at", Width: lastActiveWidth},
		}
	}

	remainingWidth := tableWidth - 24 - lastActiveWidth
	return []table.Column{
		{Title: "Agent", Width: 12},
		{Title: "Session", Width: remainingWidth / 2},
		{Title: "Directory", Width: remainingWidth - remainingWidth/2},
		{Title: "Status", Width: 12},
		{Title: "Last active at", Width: lastActiveWidth},
	}
}
