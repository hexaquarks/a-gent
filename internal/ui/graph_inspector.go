package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	graphRailMinWidth       = 170
	graphRailMinHeight      = 35
	graphRailWidth          = 34
	graphRailGap            = 3
	graphRailAttentionLimit = 3
	graphRailActivityRows   = 7
)

func (model Model) graphRailVisible() bool {
	return model.graphProjectOpen && model.width >= graphRailMinWidth && model.height >= graphRailMinHeight
}

func (model Model) graphDashboardView() string {
	width, height := model.graphDimensions()
	graph := model.graphView()
	body := graph
	if model.graphRailVisible() {
		rail := model.graphInspectorView(graphRailWidth, height)
		divider := mutedStyle.Foreground(lipgloss.Color(colorDivider)).Render(" │ ")
		divider = strings.TrimSuffix(strings.Repeat(divider+"\n", height), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(width).Render(graph), divider, rail,
		)
	}

	heading := clipLines(model.sessionHeadingView(), model.table.Width()-panelStyle.GetHorizontalFrameSize())
	return panelStyle.Width(model.table.Width()).Render(heading + strings.Repeat("\n", model.boardHeadingHeight()) + body)
}

func (model Model) graphInspectorView(width, height int) string {
	var attention []string
	count := 0
	for _, session := range model.filteredSessions() {
		if !matchesView(session, needsYouView) {
			continue
		}
		count++
		if len(attention) < graphRailAttentionLimit*2 {
			attention = append(attention,
				providerStyle(session.Provider).Render(safeDisplayText(session.Provider))+mutedStyle.Render(" · ")+
					model.projectStyle(session.WorkingDirectory).Render(safeDisplayText(projectName(session.WorkingDirectory))),
				clipLines(safeDisplayText(sessionDisplayName(session)), width),
			)
		}
	}

	lines := []string{waitingStyle.Bold(true).Render(fmt.Sprintf("NEEDS YOU · %d", count)), ""}
	if count == 0 {
		attention = append(attention, mutedStyle.Render("No agents need input"))
	}
	lines = append(lines, attention...)
	lines = append(lines, "", mutedStyle.Render(strings.Repeat("─", width)))

	lines = append(lines, accentStyle.Render("SELECTED AGENT"), "")

	session, selected := model.selectedSession()
	if selected {
		lines = append(lines,
			mainTextStyle.Bold(true).Render(clipLines(safeDisplayText(sessionDisplayName(session)), width)),
			providerStyle(session.Provider).Render(safeDisplayText(session.Provider))+mutedStyle.Render(" · ")+
				model.projectStyle(session.WorkingDirectory).Render(safeDisplayText(projectName(session.WorkingDirectory))),
			statusStyle(sessionState(session)).Render(indicatorDot+" "+displayState(sessionState(session))), "",
		)

		cached := model.previewCache[sessionIdentity{session.Provider, session.ID}]
		remaining := max(1, height-len(lines)-1)
		activityRows := min(graphRailActivityRows, remaining/2)
		activity := model.inspectorActivity(session, cached, width, activityRows)
		edit := model.inspectorEdit(session, cached, width, remaining-activityRows)
		lines = append(lines, strings.Split(activity, "\n")...)
		lines = append(lines, strings.Split(edit, "\n")...)
	}

	lines = append(lines, shortcutKeyStyle.Render("v")+mutedStyle.Render(" expand details"))
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).
		Render(clipLines(strings.Join(lines, "\n"), width))
}
