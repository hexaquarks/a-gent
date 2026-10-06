package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	if model.previewExpanded {
		return model.previewView(max(1, model.width), max(1, model.height), true)
	}
	summary := summarizeSessions(model.sessions)
	tableView := model.sessionTableView()
	main := panelStyle.Width(model.table.Width()).Render(model.sessionHeadingView() + "\n\n" + tableView)
	detail := detailStyle.PaddingRight(0).Width(model.table.Width()).Render(model.detailWithPreview())
	rightColumn := lipgloss.NewStyle().Height(model.bodyHeight()).Render(lipgloss.JoinVertical(lipgloss.Left, main, detail))
	sidebar := sidebarStyle.Height(model.bodyHeight()).Render(model.renderSidebar(summary))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightColumn)

	dataStatus := "live data"
	if model.lastError != nil {
		dataStatus = "partial data"
	}

	headerText := lipgloss.JoinHorizontal(
		lipgloss.Left,
		accentStyle.Render("a-gent"),
		mutedStyle.Render(fmt.Sprintf("  /  %d sessions  /  ", summary.total)),
		runningStyle.Render(fmt.Sprintf("%d running", summary.running)),
		mutedStyle.Render("  /  "+dataStatus),
	)
	contentWidth := lipgloss.Width(body)
	header := headerStyle.Width(contentWidth).Render(headerText)
	footer := model.footerView(contentWidth)

	view := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
	if model.width > 0 {
		view = clipLines(view, model.width)
	}
	if model.height > 0 && lipgloss.Height(view) > model.height {
		view = strings.Join(strings.Split(view, "\n")[:model.height], "\n")
	}
	return view
}

func (model Model) detailView() string {
	title := model.detailHeadingView()
	selectedSession, ok := model.selectedSession()
	if !ok {
		if model.lastError != nil {
			return title + "\n" + errorStyle.Render("Could not read live sessions.")
		}
		return title + "\n" + mutedStyle.Render("No live sessions found.")
	}

	stateText := displayState(sessionState(selectedSession))
	if selectedSession.Stale {
		stateText = "Stale · last known data"
	}

	return fmt.Sprintf(
		"%s\n%s  %s\n%s\n%s %s\n%s %s",
		title,
		providerStyle(selectedSession.Provider).Render(safeDisplayText(selectedSession.Provider)),
		statusStyle(sessionState(selectedSession)).Render("● "+safeDisplayText(stateText)),
		mainTextStyle.Render(safeDisplayText(selectedSession.Name)),
		mutedStyle.Render("Directory:"),
		mainTextStyle.Render(safeDisplayText(selectedSession.WorkingDirectory)),
		mutedStyle.Render("Session:"),
		mutedStyle.Render(safeDisplayText(selectedSession.ID)),
	)
}

func (model Model) detailHeadingView() string {
	return model.detailHeadingAtWidth(model.table.Width() - detailStyle.GetHorizontalFrameSize())
}

func (model Model) detailHeadingAtWidth(width int) string {
	title := "SELECTED SESSION"
	session, ok := model.selectedSession()
	if !ok || !model.unreadSessions[sessionIdentity{provider: session.Provider, id: session.ID}] {
		return model.panelTitleStyle().Render(title)
	}

	indicator := accentStyle.Render("●") + " " + mutedStyle.Render("Unseen state change")
	titleWidth := max(0, width-lipgloss.Width(indicator)-2)
	title = runewidth.Truncate(title, titleWidth, "…")
	gap := strings.Repeat(" ", max(0, width-lipgloss.Width(title)-lipgloss.Width(indicator)))
	return model.panelTitleStyle().Render(title) + gap + indicator
}
