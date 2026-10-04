package ui

import (
	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
)

func (model *Model) updateUnreadSessions(sessions []agent.Session) {
	previousStates := make(map[sessionIdentity]agent.State, len(model.sessions))
	for _, session := range model.sessions {
		previousStates[sessionIdentity{provider: session.Provider, id: session.ID}] = session.State
	}

	unread := make(map[sessionIdentity]bool)
	for _, session := range sessions {
		identity := sessionIdentity{provider: session.Provider, id: session.ID}
		if session.State != agent.StateRunning &&
			(model.unreadSessions[identity] || previousStates[identity] == agent.StateRunning) {
			unread[identity] = true
		}
	}
	model.unreadSessions = unread
}

func (model *Model) markSelectedSessionRead() {
	if session, ok := model.selectedSession(); ok {
		delete(model.unreadSessions, sessionIdentity{provider: session.Provider, id: session.ID})
	}
}

func (model *Model) markHoveredSessionRead(x, y int) {
	// Derive hit bounds from the rendered panels so padding, borders, and a
	// wrapped application header match the rows the user actually sees.
	sidebar, rightColumn := model.dashboardPanels()
	header := model.headerView(lipgloss.Width(sidebar) + lipgloss.Width(rightColumn))
	left := appStyle.GetPaddingLeft() + lipgloss.Width(sidebar) + panelStyle.GetPaddingLeft()
	top := appStyle.GetPaddingTop() + lipgloss.Height(header) + panelStyle.GetPaddingTop() +
		lipgloss.Height(model.sessionHeadingView()) + 3
	rowWidth := selectionCursorWidth
	for _, column := range model.table.Columns() {
		rowWidth += column.Width
	}
	start, end := model.visibleSessionRange()
	if x < left || x >= left+rowWidth || y < top || y >= top+end-start {
		return
	}

	session := model.filteredSessions()[start+y-top]
	delete(model.unreadSessions, sessionIdentity{provider: session.Provider, id: session.ID})
}
