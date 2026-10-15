package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	if model.previewExpanded {
		view := model.expandedPreviewView(max(1, model.width), max(1, model.height))
		if model.helpOpen {
			return model.helpView(view)
		}
		return view
	}
	summary := summarizeSessions(model.sessions)
	var sessionsView, detailContent string
	heading := model.sessionHeadingView()
	headingSpacing := "\n\n"
	if model.boardVisible {
		heading = clipLines(heading, model.table.Width()-panelStyle.GetHorizontalFrameSize())
		sessionsView = model.boardView()
		headingSpacing = strings.Repeat("\n", model.boardHeadingHeight())
		detailContent = model.boardFocusView(
			model.table.Width() - detailStyle.PaddingRight(0).GetHorizontalFrameSize(),
		)
	} else {
		sessionsView = model.sessionTableView()
		detailContent = model.detailWithPreview()
	}
	main := panelStyle.Width(model.table.Width()).Render(heading + headingSpacing + sessionsView)
	detail := ""
	if !model.inspectorCollapsed {
		detail = detailStyle.PaddingRight(0).Width(model.table.Width()).Render(detailContent)
	}
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
	if model.newAgent != nil {
		view = model.newAgentView(view)
	}
	if model.helpOpen {
		return model.helpView(view)
	}
	return view
}

func (model Model) expandedPreviewView(width, height int) string {
	session, selected := model.selectedSession()
	cached := model.previewCache[sessionIdentity{provider: session.Provider, id: session.ID}]
	var metadata []string
	if selected && height >= 8 {
		metadata = []string{
			mainTextStyle.Bold(true).Render(safeDisplayText(sessionDisplayName(session))),
			mutedStyle.Render("Directory: " + displayDirectory(session.WorkingDirectory)),
			mutedStyle.Render("Session: " + safeDisplayText(session.ID)),
		}
	}

	if cached.activity != nil && cached.edit != nil && height > len(metadata)+1 {
		activityStyle, editStyle := accentStyle, mutedStyle
		if model.previewShowsEdit {
			activityStyle, editStyle = mutedStyle, accentStyle
		}
		metadata = append(metadata, activityStyle.Render("Activity")+"  "+editStyle.Render("Latest edit")+
			mutedStyle.Render(" · Tab switch"))
	}

	if len(metadata) == 0 {
		return model.previewView(width, height, true)
	}

	metadata = append(metadata, "")
	preview := model.previewView(width, height-len(metadata), true)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).
		Render(clipLines(strings.Join(metadata, "\n"), width) + "\n" + preview)
}

func displayDirectory(directory string) string {
	directory = safeDisplayText(directory)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if directory == home || strings.HasPrefix(directory, home+string(filepath.Separator)) {
			directory = "~" + strings.TrimPrefix(directory, home)
		}
	}
	return directory
}

func (model Model) headerView(summary sessionSummary, width int) string {
	subagents := 0
	for _, session := range model.sessions {
		if session.ParentID != "" {
			subagents++
		}
	}
	count := fmt.Sprintf("  /  %d sessions", summary.total-subagents)
	if subagents > 0 {
		label := "subagents"
		if subagents == 1 {
			label = "subagent"
		}
		count += fmt.Sprintf(" · %d %s", subagents, label)
	}
	brand := accentStyle.Render("a-gent") + mutedStyle.Render(count)
	badges := []string{
		summaryBadge(fmt.Sprintf("%d running", summary.running), colorRunning, "#202D1C"),
		summaryBadge(fmt.Sprintf("%d needs you", summary.total-summary.running-summary.idle), colorAttention, "#30291B"),
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

// overlayPanel preserves terminal column boundaries around a centered panel.
func (model Model) overlayPanel(background, box string) string {
	boxLines := strings.Split(box, "\n")
	rows := strings.Split(background, "\n")
	height := max(1, model.height)
	for len(rows) < height {
		rows = append(rows, "")
	}
	x := max(0, (model.width-lipgloss.Width(box))/2)
	y := max(0, (height-len(boxLines))/2)
	for index, line := range boxLines {
		if y+index >= height {
			break
		}
		row := lipgloss.NewStyle().Width(max(1, model.width)).Render(rows[y+index])
		rows[y+index] = ansi.Cut(row, 0, x) + line + ansi.Cut(row, x+lipgloss.Width(box), model.width)
	}
	return clipLines(strings.Join(rows[:height], "\n"), max(1, model.width))
}
