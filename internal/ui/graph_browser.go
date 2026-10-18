package ui

import (
	"fmt"
)

// graphBrowserLayout shows one level at a time: projects around a-gent, or
// individual agents around the opened project. A card has exactly one edge.
func (model Model) graphBrowserLayout() graphLayout {
	width, height := model.graphDimensions()
	if !model.graphProjectOpen {
		return model.graphProjectMapLayout(width, max(1, height-1))
	}
	return fitGraphAgents(model.filteredSessions(), width, max(1, height-1))
}

// A project can contain several table rows; all select the same overview card.
func (model Model) graphSelectedIndex() int {
	index := model.table.Cursor()
	if model.graphProjectOpen {
		return index
	}
	sessions := model.filteredSessions()
	if index < 0 || index >= len(sessions) {
		return 0
	}
	for _, group := range graphGroups(sessions) {
		if group.directory == sessions[index].WorkingDirectory {
			return group.indexes[0]
		}
	}
	return index
}

func (model *Model) openGraphProject() {
	session, ok := model.selectedSession()
	if !ok {
		return
	}
	model.graphProjectDirectory = session.WorkingDirectory
	model.graphProjectOpen = true
	model.sidebarFocus = false
	model.updateTableRows()
}

func graphCountLabel(count int, singular string) string {
	if count != 1 {
		singular += "s"
	}
	return fmt.Sprintf("%d %s", count, singular)
}

func (model *Model) closeGraphProject() {
	model.graphProjectDirectory = ""
	model.graphProjectOpen = false
	model.updateTableRows()
}
