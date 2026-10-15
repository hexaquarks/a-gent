package ui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/x/ansi"
)

const (
	selectionCursorWidth = 4
	scrollbarGutterWidth = 2
	defaultTableWidth    = 70
	minimumTableWidth    = 36
	sidebarContentWidth  = 26
	defaultSessionRows   = 8
	compactInspectorRows = 8

	// The sidebar separator and two outer borders share the terminal width.
	sidebarLayoutFrameWidth = 3

	// PopupWidth and PopupHeight keep the dashboard's wide proportions stable
	// across terminal sizes. tmux clamps these dimensions to the available space.
	PopupWidth  = 120
	PopupHeight = 29
	// PopupContentHeight equals the popup height because the app draws its border.
	PopupContentHeight = PopupHeight
)

func (model *Model) resizeTable() {
	tableWidth := defaultTableWidth
	if model.width > 0 {
		tableWidth = model.width - sidebarContentWidth - sidebarLayoutFrameWidth
	}
	if model.width > 0 && !model.sidebarVisible() {
		tableWidth = max(8, model.width-2)
	}
	tableWidth = max(tableWidth, 8)

	// Include room for a branch before provider names when children are visible.
	providers := model.providers()
	for _, session := range model.sessions {
		if session.ParentID != "" {
			for index := range providers {
				providers[index] = "   " + providers[index]
			}
			break
		}
	}
	columns := tableColumns(tableWidth, providers...)
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
		model.sort = sessionSort{column: "Age", descending: true}
	}
	model.updateTableRows()

	tableHeight := defaultSessionRows
	if model.height > 0 {
		// Reserve the toolbar, table heading, and detail separator; the list gets the remaining rows.
		reservedRows := 4
		tableHeight = max(1, model.bodyHeight()-model.inlinePreviewHeight()-reservedRows)
	}
	model.table.SetHeight(tableHeight + 1)
}

func tableColumns(tableWidth int, providers ...string) []table.Column {
	tableWidth -= panelStyle.GetHorizontalFrameSize() + selectionCursorWidth + scrollbarGutterWidth
	if tableWidth < 27 {
		return []table.Column{{Title: "Session", Width: max(1, tableWidth)}}
	}
	// Fit the provider names, longest status, and activity heading without
	// reserving extra padding that would shorten session titles.
	const (
		statusWidth = 15
		ageWidth    = 15
	)
	agentWidth := 8
	for _, provider := range providers {
		agentWidth = max(agentWidth, min(16, ansi.StringWidth(safeDisplayText(provider))+2))
	}
	if tableWidth < 44 {
		return []table.Column{
			{Title: "Agent", Width: agentWidth},
			{Title: "Status", Width: tableWidth - agentWidth - ageWidth},
			{Title: "Age", Width: ageWidth},
		}
	}
	// Show projects only when there is also room for a readable session name.
	projectWidth := 0
	if tableWidth >= 76 {
		projectWidth = min(22, tableWidth/5)
	}
	columns := []table.Column{
		{Title: "Agent", Width: agentWidth},
		{Title: "Session", Width: tableWidth - agentWidth - projectWidth - statusWidth - ageWidth},
	}
	if projectWidth > 0 {
		columns = append(columns, table.Column{Title: "Project", Width: projectWidth})
	}
	return append(columns,
		table.Column{Title: "Status", Width: statusWidth},
		table.Column{Title: "Age", Width: ageWidth},
	)
}

// Keep the inline inspector compact; v opens the full activity or diff.
func (model Model) inlinePreviewHeight() int {
	if model.height == 0 {
		return compactInspectorRows
	}
	return min(compactInspectorRows, max(1, model.bodyHeight()-4))
}

func (model Model) sidebarVisible() bool {
	minimumWidth := sidebarContentWidth + minimumTableWidth + sidebarLayoutFrameWidth
	return (model.width == 0 || model.width >= minimumWidth) && (model.height == 0 || model.height >= 21)
}

// The board uses the space released by compact details rather than the list's
// row limit. Very short terminals retain the selected card and expansion key.
func (model Model) boardHeight() int {
	detailHeight := model.boardFocusHeight() + detailStyle.GetVerticalFrameSize()
	return max(1, model.bodyHeight()-model.boardHeadingHeight()-detailHeight)
}

func (model Model) boardHeadingHeight() int {
	if model.bodyHeight() < 7 {
		return 1
	}
	return 2
}

func (model Model) boardFocusHeight() int {
	height := 2
	if model.height >= boardFocusRoomyMinHeight ||
		model.table.Width()-detailStyle.PaddingRight(0).GetHorizontalFrameSize() < boardFocusColumnsMinWidth {
		height = 3
	}
	return min(height, max(1, model.bodyHeight()-4))
}
