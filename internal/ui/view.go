package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	rightColumn := lipgloss.NewStyle().Height(model.bodyHeight()).MaxHeight(model.bodyHeight()).Render(lipgloss.JoinVertical(lipgloss.Left, main, detail))
	sidebar := sidebarStyle.Height(model.bodyHeight()).Render(model.renderSidebar(summary))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightColumn)
	if !model.sidebarVisible() {
		body = rightColumn
	}

	contentWidth := lipgloss.Width(body)
	header := model.headerView(summary, contentWidth)
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

	directory := safeDisplayText(selectedSession.WorkingDirectory)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if directory == home || strings.HasPrefix(directory, home+string(filepath.Separator)) {
			directory = "~" + strings.TrimPrefix(directory, home)
		}
	}
	fields := []struct{ label, value string }{
		{"Agent", providerStyle(selectedSession.Provider).Bold(true).Render(safeDisplayText(selectedSession.Provider))},
		{"Status", statusStyle(sessionState(selectedSession)).Render("● " + safeDisplayText(stateText))},
		{"Project", mainTextStyle.Render(safeDisplayText(projectName(selectedSession.WorkingDirectory)))},
		{"Directory", mutedStyle.Render(directory)},
		{"Session", mutedStyle.Render(safeDisplayText(selectedSession.ID))},
	}
	lines := []string{title, "", mainTextStyle.Bold(true).Render(safeDisplayText(selectedSession.Name)), ""}
	for _, field := range fields {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("%-11s", field.label))+field.value)
	}
	return strings.Join(lines, "\n")
}

func (model Model) detailHeadingView() string {
	return model.detailHeadingAtWidth(model.table.Width() - detailStyle.GetHorizontalFrameSize())
}

func (model Model) detailHeadingAtWidth(width int) string {
	return sectionBar("SELECTED SESSION", width, model.panelTitleStyle())
}

func (model Model) headerView(summary sessionSummary, width int) string {
	brand := accentStyle.Render("a-gent") + mutedStyle.Render(fmt.Sprintf("  /  %d sessions", summary.total))
	badges := []string{
		summaryBadge(fmt.Sprintf("%d running", summary.running), colorRunning, "#202D1C"),
		summaryBadge(fmt.Sprintf("%d needs input", summary.waiting), colorAttention, "#30291B"),
		summaryBadge(fmt.Sprintf("%d unseen", len(model.unreadSessions)), colorAccent, "#152B30"),
	}
	status := runningStyle.Render("●") + mutedStyle.Render(" live")
	if model.lastError != nil {
		status = waitingStyle.Render("●") + mutedStyle.Render(" partial")
	}
	contentWidth := width - headerStyle.GetHorizontalFrameSize()
	left := brand
	for _, badge := range badges {
		candidate := left + "  " + badge
		if lipgloss.Width(candidate)+lipgloss.Width(status)+2 <= contentWidth {
			left = candidate
		}
	}
	return headerStyle.Width(width).Render(alignedLine(left, status, contentWidth))
}

func summaryBadge(text, foreground, background string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(foreground)).Background(lipgloss.Color(background)).Padding(0, 1).Render(text)
}

func alignedLine(left, right string, width int) string {
	left = clipLines(left, max(1, width-lipgloss.Width(right)-1))
	return left + strings.Repeat(" ", max(0, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
}
