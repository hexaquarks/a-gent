package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type sessionSummary struct {
	total   int
	running int
	waiting int
	idle    int
	errors  int
}

// sidebarView identifies a predefined session filter in the sidebar.
type sidebarView string

const (
	attentionView sidebarView = "Attention"
	activeView    sidebarView = "Active"
	recentView    sidebarView = "Recent"
	updatesView   sidebarView = "Updates"
	allView       sidebarView = "All"
)

type sidebarItem struct {
	// label is the user-visible name of the view or project.
	label string
	// view is set for a VIEWS item.
	view sidebarView
	// project is set for a PROJECTS item.
	project    string
	provider   string
	agentGroup bool
	allTypes   bool
}

func (model Model) renderSidebar(summary sessionSummary) string {
	items := model.sidebarItems()
	titleStyle := sectionStyle
	if model.sidebarFocus {
		titleStyle = accentStyle
	}
	lines := []string{sectionBar(" VIEWS", model.sidebarWidth(), titleStyle)}
	for index, item := range items[:len(sidebarViews())] {
		lines = append(lines, model.sidebarItemView(item, index, model.viewCount(item.view, summary)))
	}

	// Reserve the same project area even when filtering leaves fewer rows.
	projectTop := model.projectTop()
	providers := model.providers()
	if len(providers) > 0 {
		title := "AGENTS"
		if len(providers) > 3 {
			arrow := "▸"
			if model.agentsExpanded {
				arrow = "▾"
			}
			title = fmt.Sprintf("AGENTS (%d) %s", len(providers), arrow)
		}
		lines = append(lines, "", sectionBar(" "+title, model.sidebarWidth(), titleStyle))
		start := len(sidebarViews())
		end := model.projectItemStart()
		available := max(1, projectTop-len(lines)-1-len(model.failedProviders()))
		if end-start > available {
			// Keep the focused type visible when an expanded list needs to scroll.
			start = min(max(start, model.sidebarCursor-available+1), end-available)
		}
		for index := start; index < min(end, start+available); index++ {
			item := items[index]
			lines = append(lines, model.sidebarItemView(item, index, model.providerCount(item.provider)))
		}
	}
	for _, provider := range model.failedProviders() {
		lines = append(lines, errorStyle.Render(runewidth.Truncate(safeDisplayText(provider+": unavailable"), model.sidebarWidth(), "…")))
	}
	for len(lines) < projectTop {
		lines = append(lines, "")
	}
	lines = append(lines, sectionBar(fmt.Sprintf(" PROJECTS (%d)", len(model.projects())), model.sidebarWidth(), titleStyle))

	start, end := model.visibleProjectRange()
	for index := start; index < end; index++ {
		itemIndex := model.projectItemStart() + index
		lines = append(lines, model.sidebarItemView(items[itemIndex], itemIndex, 0))
	}
	if model.projectSearching && start == end {
		lines = append(lines, mutedStyle.Render("No matching projects"))
	}
	for len(lines) < model.sidebarHeight()-2 {
		lines = append(lines, "")
	}
	hidden := len(model.matchingProjects()) - (end - start)
	if hidden > 0 {
		lines = append(lines, accentStyle.Render(fmt.Sprintf("+ %d more…", hidden)))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(colorDivider)).Render(strings.Repeat("─", model.sidebarWidth())))
	}
	if model.projectSearching {
		lines = append(lines, accentStyle.Render("/ "+runewidth.Truncate(model.projectQuery, model.sidebarWidth()-2, "…")))
	} else {
		lines = append(lines, shortcutKeyStyle.Render("/")+mutedStyle.Render(" find · ")+shortcutKeyStyle.Render("p")+mutedStyle.Render(" pin"))
	}
	return strings.Join(lines, "\n")
}

func (model Model) sidebarWidth() int {
	return sidebarContentWidth - sidebarStyle.GetHorizontalPadding()
}

func (model Model) sidebarHeight() int {
	return model.bodyHeight() - sidebarStyle.GetVerticalFrameSize()
}

// Keep the body height stable when the selected-session details are empty.
func (model Model) bodyHeight() int {
	if model.height > 0 {
		return max(1, model.height-6)
	}
	return maximumSessionRows + model.inlinePreviewHeight() + 8
}

func sidebarViews() []sidebarView {
	return []sidebarView{attentionView, activeView, recentView, updatesView, allView}
}

func (model Model) sidebarItems() []sidebarItem {
	views := sidebarViews()
	items := make([]sidebarItem, 0, len(views)+len(model.projects()))
	for _, view := range views {
		items = append(items, sidebarItem{label: string(view), view: view})
	}
	providers := model.providers()
	if len(providers) > 3 {
		items = append(items, sidebarItem{label: "All types", agentGroup: true, allTypes: true})
	}
	if len(providers) <= 3 || model.agentsExpanded {
		for _, provider := range providers {
			items = append(items, sidebarItem{label: provider, provider: provider})
		}
	}
	for _, project := range model.matchingProjects() {
		items = append(items, sidebarItem{label: projectName(project), project: project})
	}
	return items
}

func (model Model) sidebarItemView(item sidebarItem, index, count int) string {
	selected := (item.view != "" && item.view == model.selectedView) ||
		(item.project != "" && item.project == model.selectedProject) ||
		(item.provider != "" && item.provider == model.selectedProvider)
	focused := model.sidebarFocus && index == model.sidebarCursor
	if item.view == "" && item.provider == "" && !item.allTypes {
		return model.projectItemView(item, focused, selected)
	}
	style := mutedStyle
	if item.provider != "" {
		style = providerStyle(item.provider)
	} else if selected {
		style = accentStyle
	}
	marker := " "
	if selected {
		marker = "●"
	}
	if focused {
		marker = "›"
		style = style.Background(lipgloss.Color(colorSelection))
	}
	countText := fmt.Sprint(count)
	if item.allTypes {
		countText = ""
	}
	countStyle := style
	if item.view == attentionView {
		countStyle = waitingStyle
	}
	if item.view == activeView {
		countStyle = runningStyle
	}
	if selected {
		countStyle = style
	}
	if selected {
		style = style.Background(lipgloss.Color(colorSelection))
		countStyle = countStyle.Background(lipgloss.Color(colorSelection))
	}
	if focused {
		countStyle = countStyle.Background(lipgloss.Color(colorSelection))
	}
	return model.renderSidebarRow(item.label, marker, " ", countText, style, style, countStyle)
}

// Every row reserves the same gutter, pin slot, and count column before
// allocating space to the name. Larger totals widen all count cells together.
func (model Model) renderSidebarRow(name, marker, pin, count string, style, markerStyle, countStyle lipgloss.Style) string {
	const gutterWidth = 3
	const pinWidth = 1
	const nameToPinSpacing = 1
	countWidth := max(3, len(fmt.Sprint(len(model.sessions))))
	nameWidth := model.sidebarWidth() - gutterWidth - pinWidth - nameToPinSpacing - countWidth - 1
	label := sidebarLabel(name, max(0, nameWidth))
	return markerStyle.Render(" "+marker+" ") +
		style.Render(label+strings.Repeat(" ", nameToPinSpacing)) +
		countStyle.Render(fmt.Sprintf("%*s", countWidth, count)) +
		style.Render(" "+pin)
}

// Pad using display cells so Unicode names share the same count column.
func sidebarLabel(label string, width int) string {
	label = runewidth.Truncate(safeDisplayText(label), width, "…")
	return label + strings.Repeat(" ", max(0, width-lipgloss.Width(label)))
}

func (model Model) viewCount(view sidebarView, summary sessionSummary) int {
	switch view {
	case attentionView:
		return summary.waiting + summary.errors
	case activeView:
		return summary.running
	case recentView:
		return summary.total - summary.running
	case updatesView:
		return len(model.unreadSessions)
	default:
		return summary.total
	}
}

func (model Model) projects() []string {
	projects := make(map[string]struct{})
	for _, session := range model.sessions {
		projects[session.WorkingDirectory] = struct{}{}
	}

	projectNames := make([]string, 0, len(projects))
	for project := range projects {
		projectNames = append(projectNames, project)
	}
	slices.SortFunc(projectNames, func(left, right string) int {
		if model.pinnedProjects[left] != model.pinnedProjects[right] {
			if model.pinnedProjects[left] {
				return -1
			}
			return 1
		}
		return strings.Compare(left, right)
	})
	return projectNames
}

func projectName(project string) string {
	if project == "" {
		return "Unknown"
	}
	return filepath.Base(project)
}

func (model Model) projectCount(project string) int {
	count := 0
	for _, session := range model.sessions {
		if session.WorkingDirectory == project {
			count++
		}
	}
	return count
}

func (model *Model) moveSidebarCursor(offset int) {
	items := model.sidebarItems()
	if len(items) == 0 {
		return
	}

	model.sidebarCursor = (model.sidebarCursor + offset + len(items)) % len(items)
	model.previewSidebarItem()
	model.updateTableRows()
}

func (model *Model) previewSidebarItem() {
	items := model.sidebarItems()
	if model.sidebarCursor >= 0 && model.sidebarCursor < len(items) {
		item := items[model.sidebarCursor]
		model.sidebarPreview = &item
	}
}

// Preview against the confirmed filters without changing them. Leaving the
// sidebar or pressing Enter discards this temporary selection.
func (model Model) effectiveFilters() (sidebarView, string, string) {
	view, provider, project := model.selectedView, model.selectedProvider, model.selectedProject
	if model.sidebarFocus && !model.projectSearching && model.sidebarPreview != nil {
		item := model.sidebarPreview
		switch {
		case item.view != "":
			view, project = item.view, ""
		case item.provider != "":
			provider = item.provider
		case item.allTypes:
			provider = ""
		default:
			project = item.project
		}
	}
	return view, provider, project
}

func (model *Model) clearMissingProjectFilter() {
	if model.selectedProject == "" {
		return
	}
	for _, project := range model.projects() {
		if project == model.selectedProject {
			return
		}
	}
	model.selectedProject = ""
	model.selectedView = allView
}

func (model Model) filteredSessions() []agent.Session {
	view, provider, project := model.effectiveFilters()
	filteredSessions := make([]agent.Session, 0, len(model.sessions))
	for _, session := range model.sessions {
		if provider != "" && session.Provider != provider {
			continue
		}
		if project != "" && session.WorkingDirectory != project {
			continue
		}
		if project == "" && view == updatesView && !model.unreadSessions[sessionIdentity{provider: session.Provider, id: session.ID}] {
			continue
		}
		if project == "" && !matchesView(session, view) {
			continue
		}
		filteredSessions = append(filteredSessions, session)
	}
	model.sortSessions(filteredSessions)
	return filteredSessions
}

func matchesView(session agent.Session, view sidebarView) bool {
	switch view {
	case attentionView:
		return sessionState(session) == agent.StateWaiting || sessionState(session) == agent.StateError || sessionState(session) == agent.StateUnavailable
	case activeView:
		return sessionState(session) == agent.StateRunning
	case recentView:
		// Keep sessions that have stopped running, including requests for input.
		return sessionState(session) != agent.StateRunning
	default:
		return true
	}
}

func summarizeSessions(sessions []agent.Session) sessionSummary {
	summary := sessionSummary{total: len(sessions)}
	for _, session := range sessions {
		switch sessionState(session) {
		case agent.StateRunning:
			summary.running++
		case agent.StateWaiting:
			summary.waiting++
		case agent.StateIdle:
			summary.idle++
		case agent.StateError, agent.StateUnavailable:
			summary.errors++
		}
	}

	return summary
}

func (model Model) providers() []string {
	providers := make(map[string]bool)
	for _, session := range model.sessions {
		if session.Provider != "" {
			providers[session.Provider] = true
		}
	}
	names := make([]string, 0, len(providers))
	for provider := range providers {
		names = append(names, provider)
	}
	slices.Sort(names)
	return names
}

func (model Model) providerCount(provider string) int {
	count := 0
	for _, session := range model.sessions {
		if session.Provider == provider {
			count++
		}
	}
	return count
}
