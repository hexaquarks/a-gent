package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	summary := summarizeSessions(model.sessions)
	tableView := model.sessionTableView()
	main := panelStyle.Width(model.table.Width()).Render(model.sessionHeadingView() + "\n\n" + tableView)
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
	title := model.panelTitleStyle().Render("SELECTED SESSION") + "  " +
		accentStyle.Render("●") + " " + mutedStyle.Render("Unseen state change")
	selectedSession, ok := model.selectedSession()
	if !ok {
		if model.lastError != nil {
			return title + "\n" + errorStyle.Render("Could not read live sessions.")
		}
		return title + "\n" + mutedStyle.Render("No live sessions found.")
	}

	return fmt.Sprintf(
		"%s\n%s  %s\n%s\n%s %s\n%s %s",
		title,
		agentStyle.Render(selectedSession.Provider),
		statusStyle(selectedSession.State).Render("● "+displayState(selectedSession.State)),
		mainTextStyle.Render(selectedSession.Name),
		mutedStyle.Render("Directory:"),
		mainTextStyle.Render(selectedSession.WorkingDirectory),
		mutedStyle.Render("Session:"),
		mutedStyle.Render(selectedSession.ID),
	)
}
