package ui

import (
	"fmt"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	graphProjectMapWideWidth      = 44
	graphProjectMapMediumWidth    = 34
	graphProjectMapCompactWidth   = 24
	graphProjectMapMinimumWidth   = 18
	graphProjectMapFullAgents     = 3
	graphProjectMapMediumAgents   = 2
	graphProjectMapCompactAgents  = 1
	graphProjectMapFrameRows      = 4 // Border, project heading, and separator.
	graphProjectMapWorkerRows     = 2 // Separator and worker totals.
	graphProjectMapStatusMinWidth = 28
)

type graphProjectSummary struct {
	sessions                                []agent.Session
	workers, runningWorkers, waitingWorkers int
}

func (model Model) graphProjectSummary(directory string, sessions []agent.Session) graphProjectSummary {
	summary := graphProjectSummary{}
	for _, group := range graphGroups(sessions) {
		if group.directory != directory {
			continue
		}
		for _, index := range group.indexes {
			session := sessions[index]
			summary.sessions = append(summary.sessions, session)
			total, running, waiting := model.boardWorkerCounts(session)
			summary.workers += total
			summary.runningWorkers += running
			summary.waitingWorkers += waiting
		}
		break
	}
	return summary
}

func (model Model) graphProjectMapLayout(width, height int) graphLayout {
	sessions := model.filteredSessions()
	groups := graphGroups(sessions)
	summaries := make(map[string]graphProjectSummary, len(groups))
	for _, group := range groups {
		parents := make([]agent.Session, 0, len(group.indexes))
		for _, index := range group.indexes {
			parents = append(parents, sessions[index])
		}
		summaries[group.directory] = model.graphProjectSummary(group.directory, parents)
	}

	sizes := [][2]int{
		{graphProjectMapWideWidth, graphProjectMapFullAgents},
		{graphProjectMapMediumWidth, graphProjectMapMediumAgents},
		{graphProjectMapCompactWidth, graphProjectMapCompactAgents},
		{graphProjectMapMinimumWidth, graphProjectMapCompactAgents},
	}
	var best graphLayout
	for _, size := range sizes {
		var cards []graphBranch
		for _, group := range groups {
			summary := summaries[group.directory]
			visible := min(len(group.indexes), size[1])
			rows := graphProjectMapFrameRows + visible
			if len(group.indexes) > visible {
				rows++
			}
			if summary.workers > 0 {
				rows += graphProjectMapWorkerRows
			}
			cards = append(cards, graphBranch{
				directory: group.directory, representativeIndex: group.indexes[0],
				width: size[0], height: rows, visibleAgents: visible,
			})
		}
		layout := arrangeProjectMap(cards, width, height)
		if len(layout.pages) == 0 {
			continue
		}
		if len(best.pages) == 0 || len(layout.pages) < len(best.pages) {
			best = layout
		}
		if len(best.pages) == 1 {
			break
		}
	}
	return best
}

func (model Model) graphProjectView(branch graphBranch, sessions []agent.Session) string {
	summary := model.graphProjectSummary(branch.directory, sessions)
	width := branch.width - graphNodeBorderWidth - 2*graphNodePadding
	background := lipgloss.Color(colorBackground)
	if selected, ok := model.selectedSession(); ok && selected.WorkingDirectory == branch.directory {
		background = lipgloss.Color(colorSelection)
	}
	color := model.projectStyle(branch.directory).GetForeground()
	projectStyle := lipgloss.NewStyle().Foreground(color).Background(background)
	dim := mutedStyle.Background(background)
	total := len(summary.sessions) + summary.workers
	count := graphCountLabel(total, "agent")
	nameWidth := max(1, width-ansi.StringWidth(count)-1)
	heading := projectStyle.Bold(true).Render(clipLines(safeDisplayText(projectName(branch.directory)), nameWidth))
	heading = graphProjectLine(heading, dim.Render(count), width, background)
	lines := []string{heading, dim.Render(strings.Repeat("─", width))}

	visible := min(len(summary.sessions), max(1, branch.visibleAgents))
	for _, session := range summary.sessions[:visible] {
		status := statusStyle(sessionState(session)).Background(background)
		title := status.Render(indicatorDot+" ") + mainTextStyle.Background(background).
			Render(safeDisplayText(sessionDisplayName(session)))
		if branch.width >= graphProjectMapStatusMinWidth {
			label := status.Render(displayState(sessionState(session)))
			titleWidth := max(1, width-ansi.StringWidth(label)-1)
			lines = append(lines, graphProjectLine(clipLines(title, titleWidth), label, width, background))
		} else {
			lines = append(lines, graphProjectLine(clipLines(title, width), "", width, background))
		}
	}
	if remaining := len(summary.sessions) - visible; remaining > 0 {
		label := "session"
		if remaining != 1 {
			label += "s"
		}
		more := dim.Render(fmt.Sprintf("+%d more %s · Enter", remaining, label))
		lines = append(lines, graphProjectLine(more, "", width, background))
	}
	if summary.workers > 0 {
		idle := summary.workers - summary.runningWorkers - summary.waitingWorkers
		state := fmt.Sprintf("%d idle", idle)
		stateStyle := dim
		if summary.waitingWorkers > 0 {
			state = fmt.Sprintf("%d need input", summary.waitingWorkers)
			stateStyle = waitingStyle.Background(background)
		} else if summary.runningWorkers > 0 {
			state = fmt.Sprintf("%d running", summary.runningWorkers)
			stateStyle = runningStyle.Background(background)
		}
		workers := graphCountLabel(summary.workers, "worker")
		lines = append(lines, dim.Render(strings.Repeat("─", width)),
			graphProjectLine(clipLines(dim.Render(workers+" · ")+stateStyle.Render(state), width), "", width, background),
		)
	}
	return lipgloss.NewStyle().Width(branch.width-graphNodeBorderWidth).
		Padding(0, graphNodePadding).Height(branch.height - graphNodeBorderWidth).
		Background(background).Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Render(clipLines(strings.Join(lines, "\n"), width))
}

// Style the gap too: nested colored labels reset their background, so plain
// padding would otherwise leave dark stripes across a selected project.
func graphProjectLine(left, right string, width int, background lipgloss.Color) string {
	leftWidth := width
	if right != "" {
		leftWidth -= ansi.StringWidth(right) + 1
	}
	left = clipLines(left, max(1, leftWidth))
	gap := max(0, width-ansi.StringWidth(left)-ansi.StringWidth(right))
	padding := lipgloss.NewStyle().Background(background).Render(strings.Repeat(" ", gap))
	return left + padding + right
}
