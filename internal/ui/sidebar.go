package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
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
	viewsTitleStyle := sectionStyle
	projectsTitleStyle := sectionStyle
	if model.sidebarFocus {
		viewsTitleStyle = accentStyle
		projectsTitleStyle = accentStyle
	}
	lines := []string{viewsTitleStyle.Render("VIEWS")}
	for index, item := range items[:len(sidebarViews())] {
		lines = append(lines, model.sidebarItemView(item, index, model.viewCount(item.view, summary)))
	}

	providers := model.providers()
	if len(providers) > 0 {
		title := "AGENTS"
		if len(providers) > 3 {
			arrow := "▸"
			if model.agentsExpanded {
				arrow = "▾"
			}
			title = fmt.Sprintf("%s AGENTS (%d)", arrow, len(providers))
		}
		lines = append(lines, "", viewsTitleStyle.Render(title))
	}
	projectsStarted := false
	for index, item := range items[len(sidebarViews()):] {
		count := 0
		if item.project != "" || (item.provider == "" && !item.agentGroup && !item.allTypes) {
			if !projectsStarted {
				lines = append(lines, "", projectsTitleStyle.Render("PROJECTS"))
				projectsStarted = true
			}
			count = model.projectCount(item.project)
		} else if item.provider != "" {
			count = model.providerCount(item.provider)
		}
		lines = append(lines, model.sidebarItemView(item, len(sidebarViews())+index, count))
	}
	if !projectsStarted {
		lines = append(lines, "", projectsTitleStyle.Render("PROJECTS"))
	}

	if model.lastError != nil {
		lines = append(lines, "", errorStyle.Render("● Unable to refresh"))
	}

	return strings.Join(lines, "\n")
}

func sidebarViews() []sidebarView {
	return []sidebarView{attentionView, activeView, recentView, allView}
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
	for _, project := range model.projects() {
		items = append(items, sidebarItem{label: projectName(project), project: project})
	}
	return items
}

func (model Model) sidebarItemView(item sidebarItem, index, count int) string {
	label := fmt.Sprintf("%-14s %d", item.label, count)
	if item.allTypes {
		label = "All types"
	}
	selected := (item.view != "" && item.view == model.selectedView) ||
		(item.project != "" && item.project == model.selectedProject) ||
		(item.provider != "" && item.provider == model.selectedProvider)
	focused := model.sidebarFocus && index == model.sidebarCursor

	if item.provider != "" {
		prefix := "  "
		style := providerStyle(item.provider)
		if focused {
			prefix = "› "
			style = style.Background(lipgloss.Color(colorSelection))
		} else if selected {
			prefix = "• "
		}
		return style.Render(prefix + label)
	}
	if focused {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorMainText)).Background(lipgloss.Color(colorSelection)).Render("› " + label)
	}
	if selected {
		return accentStyle.Render("• " + label)
	}
	return mutedStyle.Render("  " + label)
}

func (model Model) viewCount(view sidebarView, summary sessionSummary) int {
	switch view {
	case attentionView:
		return summary.waiting + summary.errors
	case activeView:
		return summary.running
	case recentView:
		return summary.idle
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
	slices.Sort(projectNames)
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
	selectedItem := items[model.sidebarCursor]
	if selectedItem.view != "" {
		model.selectedView = selectedItem.view
		model.selectedProject = ""
	} else if selectedItem.provider != "" {
		model.selectedProvider = selectedItem.provider
	} else if selectedItem.allTypes {
		model.selectedProvider = ""
	} else {
		model.selectedProject = selectedItem.project
	}
	model.updateTableRows()
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
	filteredSessions := make([]agent.Session, 0, len(model.sessions))
	for _, session := range model.sessions {
		if model.selectedProvider != "" && session.Provider != model.selectedProvider {
			continue
		}
		if model.selectedProject != "" && session.WorkingDirectory != model.selectedProject {
			continue
		}
		if model.selectedProject == "" && !matchesView(session, model.selectedView) {
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
		return session.State == agent.StateWaiting || session.State == agent.StateError || session.State == agent.StateUnavailable
	case activeView:
		return session.State == agent.StateRunning
	case recentView:
		// The provider has no activity timestamp. Idle sessions are the completed
		// sessions available to represent the recent view.
		return session.State == agent.StateIdle
	default:
		return true
	}
}

func summarizeSessions(sessions []agent.Session) sessionSummary {
	summary := sessionSummary{total: len(sessions)}
	for _, session := range sessions {
		switch session.State {
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
