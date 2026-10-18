package ui

import (
	"fmt"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	graphMinimumHeight   = 7
	graphMinimumWidth    = 30
	graphNodeBorderWidth = 2
	graphNodePadding     = 1
)

// graphCanvas composes already-styled text by terminal columns, including
// wide Unicode titles. All writes are clipped to the viewport.
type graphCanvas struct {
	rows  []string
	width int
}

func newGraphCanvas(width, height int) graphCanvas {
	canvas := graphCanvas{width: width, rows: make([]string, height)}
	for index := range canvas.rows {
		canvas.rows[index] = strings.Repeat(" ", width)
	}
	return canvas
}

func (canvas *graphCanvas) put(x, y int, text string) {
	if y < 0 || y >= len(canvas.rows) || x < 0 || x >= canvas.width {
		return
	}

	text = ansi.Truncate(text, canvas.width-x, "")
	end := x + ansi.StringWidth(text)
	canvas.rows[y] = ansi.Cut(canvas.rows[y], 0, x) + text + ansi.Cut(canvas.rows[y], end, canvas.width)
}

func (model Model) graphDimensions() (int, int) {
	width := max(1, model.table.Width()-panelStyle.GetHorizontalFrameSize())
	if model.graphRailVisible() {
		return width - graphRailWidth - graphRailGap, model.bodyHeight() - model.boardHeadingHeight()
	}
	return width, max(1, model.bodyHeight()-model.boardHeadingHeight())
}

func (model Model) graphView() string {
	width, height := model.graphDimensions()
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(
			accentStyle.Render("a-gent") + "\n" + mutedStyle.Render(clipLines(model.emptySessionMessage(), width)),
		)
	}

	if width < graphMinimumWidth || height < graphMinimumHeight {
		return model.graphSmallView(sessions, width, height)
	}

	// Reserve a row for explicit scale and overflow information.
	layout := model.graphBrowserLayout()
	if len(layout.pages) == 0 {
		return model.graphSmallView(sessions, width, height)
	}
	pageIndex := layout.selectedPage(model.graphSelectedIndex())
	page := layout.pages[pageIndex]
	canvas := newGraphCanvas(width, height)
	root := graphNode{x: layout.rootX, y: layout.rootY, width: graphCenterWidth, height: graphRootHeight}
	canvas.drawGraphEdges(model, page, root)

	for _, branch := range page.branches {
		if len(branch.nodes) == 0 {
			project := model.graphProjectView(branch, sessions)
			for offset, line := range strings.Split(project, "\n") {
				canvas.put(branch.x, branch.y+offset, line)
			}
		}
		for _, node := range branch.nodes {
			view := model.graphNodeView(sessions[node.index], node.index, node.width, node.height)
			for offset, line := range strings.Split(view, "\n") {
				canvas.put(node.x, node.y+offset, line)
			}
		}
	}
	rootTitle := "a-gent"
	if model.graphProjectOpen {
		rootTitle = clipLines(safeDisplayText(projectName(model.graphProjectDirectory)), graphCenterWidth-2)
	}
	rootView := lipgloss.NewStyle().Width(graphCenterWidth - 2).Align(lipgloss.Center).
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(colorAccent)).
		Foreground(lipgloss.Color(colorAccent)).Bold(true).Render(rootTitle)
	for offset, line := range strings.Split(rootView, "\n") {
		canvas.put(root.x, root.y+offset, line)
	}

	caption := graphCountLabel(len(graphGroups(sessions)), "project")
	if !model.graphProjectOpen {
		if selected, ok := model.selectedSession(); ok {
			caption += " · " + safeDisplayText(projectName(selected.WorkingDirectory))
		}
	}
	if model.graphProjectOpen {
		caption = graphCountLabel(len(sessions), "agent")
	}
	if len(layout.pages) > 1 {
		caption += fmt.Sprintf(" · page %d/%d · [ ]", pageIndex+1, len(layout.pages))
	}
	if model.graphProjectOpen {
		caption += " · q/Esc projects · Enter open agent"
	} else {
		caption += " · Enter open · h/j/k/l select"
	}

	canvas.put(0, height-1, mutedStyle.Render(clipLines(caption, width)))
	return strings.Join(canvas.rows, "\n")
}

func (model Model) graphNodeView(session agent.Session, index, width, height int) string {
	unread := ""
	if model.boardSessionUnread(session) {
		unread = " •"
	}
	background := lipgloss.Color(colorBackground)
	if index == model.table.Cursor() {
		background = lipgloss.Color(colorSelection)
	}
	title := safeDisplayText(sessionDisplayName(session))
	stateStyle := statusStyle(sessionState(session)).Background(background)
	status := stateStyle.Render(indicatorDot + " " + displayState(sessionState(session)))
	provider := providerStyle(session.Provider).Background(background).Render(safeDisplayText(session.Provider))
	contentWidth := max(1, width-graphNodeBorderWidth-2*graphNodePadding)
	lines := []string{clipLines(provider+unread, contentWidth)}
	if height >= graphTinyNodeRows {
		lines = append(lines, clipLines(title, contentWidth))
	}
	if height == graphCompactNodeRows {
		lines = append(lines, clipLines(status, contentWidth))
	}
	if height == graphFullNodeRows {
		wrapped := strings.Split(ansi.Wrap(title, contentWidth, ""), "\n")
		titleLines := []string{"", ""}
		for index := 0; index < min(len(wrapped), len(titleLines)); index++ {
			titleLines[index] = clipLines(wrapped[index], contentWidth)
		}
		lines = append(lines[:1], titleLines...)
		lines = append(lines, clipLines(status, contentWidth))
	}

	border := statusStyle(sessionState(session)).GetForeground()
	return lipgloss.NewStyle().Width(width-graphNodeBorderWidth).Padding(0, graphNodePadding).Background(background).
		Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Render(strings.Join(lines, "\n"))
}

func (model Model) graphSmallView(sessions []agent.Session, width, height int) string {
	session := sessions[model.table.Cursor()]
	title := accentStyle.Render("a-gent → ") + model.projectStyle(session.WorkingDirectory).Render(
		safeDisplayText(projectName(session.WorkingDirectory)),
	)
	var content string
	if model.graphProjectOpen {
		node := model.graphNodeView(session, model.table.Cursor(), max(3, width), graphCompactNodeRows)
		content = title + "\n" + node
	} else {
		summary := model.graphProjectSummary(session.WorkingDirectory, sessions)
		count := len(summary.sessions) + summary.workers
		content = title + "\n" + graphCountLabel(count, "agent") + " · Enter open"
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(clipLines(content, width))
}
