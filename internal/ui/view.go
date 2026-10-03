package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	summary := summarizeSessions(model.sessions)
	title := model.panelTitleStyle().Render(model.sessionTitle())
	main := panelStyle.Width(model.table.Width()).Render(title + "\n\n" + model.sessionTableView())
	detail := detailStyle.Width(model.table.Width()).Render(model.detailView())
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, main, detail)
	sidebar := sidebarStyle.Height(lipgloss.Height(rightColumn)).Render(model.renderSidebar(summary))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightColumn)

	headerText := lipgloss.JoinHorizontal(
		lipgloss.Left,
		accentStyle.Render("a-gent"),
		mutedStyle.Render(fmt.Sprintf("  /  %d sessions  /  ", summary.total)),
		runningStyle.Render(fmt.Sprintf("%d running", summary.running)),
		mutedStyle.Render("  /  live data"),
	)
	contentWidth := lipgloss.Width(body)
	header := headerStyle.Width(contentWidth).Render(headerText)
	footer := model.footerView(contentWidth)

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}

func (model Model) detailView() string {
	selectedSession, ok := model.selectedSession()
	if !ok {
		if model.lastError != nil {
			return model.panelTitleStyle().Render("SELECTED SESSION") + "\n" + errorStyle.Render("Could not read live sessions.")
		}
		return model.panelTitleStyle().Render("SELECTED SESSION") + "\n" + mutedStyle.Render("No live sessions found.")
	}

	return fmt.Sprintf(
		"%s\n%s  %s\n%s\n%s %s\n%s %s",
		model.panelTitleStyle().Render("SELECTED SESSION"),
		agentStyle.Render(selectedSession.Provider),
		statusStyle(selectedSession.State).Render("● "+displayState(selectedSession.State)),
		mainTextStyle.Bold(true).Render(selectedSession.Name),
		mutedStyle.Render("Directory:"),
		mainTextStyle.Render(selectedSession.WorkingDirectory),
		mutedStyle.Render("Session:"),
		mutedStyle.Render(selectedSession.ID),
	)
}
