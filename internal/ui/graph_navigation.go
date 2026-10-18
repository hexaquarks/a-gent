package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (model *Model) moveGraphPage(direction int) {
	layout := model.graphBrowserLayout()
	if len(layout.pages) == 0 {
		return
	}
	page := layout.selectedPage(model.graphSelectedIndex())
	next := max(0, min(len(layout.pages)-1, page+direction))
	if next != page {
		targets := graphPageTargets(layout.pages[next])
		model.table.SetCursor(targets[0].index)
	}
}

func (model *Model) selectGraphNumber(number int) {
	layout := model.graphBrowserLayout()
	if len(layout.pages) == 0 {
		return
	}
	page := layout.pages[layout.selectedPage(model.graphSelectedIndex())]
	targets := graphPageTargets(page)
	if number < len(targets) {
		model.table.SetCursor(targets[number].index)
	}
}

// Movement follows the rendered cards on the current page. Prefer a card
// directly in the requested row or column before considering diagonal cards.
// Paging is explicit so reaching an edge never jumps to an unrelated layout.
func (model *Model) moveGraphCursor(horizontal, vertical int) {
	layout := model.graphBrowserLayout()
	if len(layout.pages) == 0 {
		return
	}
	selected := model.graphSelectedIndex()
	targets := graphPageTargets(layout.pages[layout.selectedPage(selected)])
	for _, current := range targets {
		if current.index == selected {
			model.table.SetCursor(graphDirectionalTarget(current, targets, horizontal, vertical))
			return
		}
	}
}

func (model Model) graphOrigin() (int, int) {
	x := appStyle.GetBorderLeftSize() + panelStyle.GetPaddingLeft()
	if model.sidebarVisible() {
		x += lipgloss.Width(sidebarStyle.Render(""))
	}
	y := appStyle.GetBorderTopSize() + 1 + headerStyle.GetVerticalFrameSize() + model.boardHeadingHeight()
	return x, y
}

func (model Model) handleGraphClick(message tea.MouseMsg) Model {
	if !model.graphVisible || model.helpOpen || model.previewExpanded || model.newAgent != nil || model.projectSearching {
		return model
	}
	if message.Button != tea.MouseButtonLeft || message.Action != tea.MouseActionPress {
		return model
	}
	layout := model.graphBrowserLayout()
	if len(layout.pages) == 0 {
		return model
	}
	originX, originY := model.graphOrigin()
	x, y := message.X-originX, message.Y-originY
	page := layout.pages[layout.selectedPage(model.graphSelectedIndex())]
	for _, target := range graphPageTargets(page) {
		if x < target.x || x >= target.x+target.width || y < target.y || y >= target.y+target.height {
			continue
		}
		model.sidebarFocus = false
		model.sidebarPreview = nil
		model.table.SetCursor(target.index)
		if !model.graphProjectOpen {
			model.openGraphProject()
		}
		return model
	}
	// Clicking the project hub is the mouse equivalent of Esc.
	if model.graphProjectOpen && x >= layout.rootX && x < layout.rootX+graphCenterWidth &&
		y >= layout.rootY && y < layout.rootY+graphRootHeight {
		model.closeGraphProject()
	}
	return model
}
