// Package ui renders the a-gent dashboard.
package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	requestTimeout = 2 * time.Second
)

// Model holds the UI state for the application.
type Model struct {
	previewSources   map[string]agent.PreviewSource
	previewCache     map[sessionIdentity]previewEntry
	previewKey       sessionIdentity
	previewRevision  int
	previewCancel    context.CancelFunc
	previewExpanded  bool
	previewScroll    int
	table            table.Model
	updates          <-chan polling.Update
	providerErrors   map[string]error
	sessions         []agent.Session
	navigator        SessionNavigator
	selectedView     sidebarView
	selectedProject  string
	selectedProvider string
	agentsExpanded   bool
	pinnedProjects   map[string]bool
	projectPinsPath  string
	projectSearching bool
	projectQuery     string
	sidebarFocus     bool
	sidebarCursor    int
	sidebarPreview   *sidebarItem
	lastError        error
	notice           string
	noticeRevision   int
	width            int
	height           int
	sort             sessionSort
	tableSessionIDs  []sessionIdentity
	unreadSessions   map[sessionIdentity]bool

	appContext       context.Context
	navigationCancel context.CancelFunc
}

type sessionNavigationMessage struct {
	err error
}

type noticeExpiredMessage struct {
	revision int
}

// NewModel creates a dashboard that consumes independently refreshed providers.
func NewModel(updates <-chan polling.Update, options ...ModelOption) Model {
	agentTable := table.New(
		table.WithColumns(tableColumns(defaultTableWidth)),
		table.WithFocused(true),
		table.WithHeight(maximumSessionRows+1),
		table.WithWidth(defaultTableWidth),
	)

	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		Bold(true).
		Foreground(lipgloss.Color("#BDBDBD")).
		Background(lipgloss.Color("#202020"))
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#292929"))
	agentTable.SetStyles(styles)

	model := Model{
		table:          agentTable,
		appContext:     context.Background(),
		updates:        updates,
		providerErrors: make(map[string]error),
		selectedView:   allView,
		sort:           sessionSort{column: "Last active", descending: true},
	}
	for _, option := range options {
		option(&model)
	}

	return model
}

// Init waits for the first provider update.
func (model Model) Init() tea.Cmd {
	if len(model.previewSources) == 0 {
		return awaitProviderUpdate(model.updates)
	}
	return tea.Batch(awaitProviderUpdate(model.updates), previewTimer())
}

// update handles dashboard events; Update also synchronizes the selected preview.
func (model Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		model.resizeTable()
		return model, nil
	case tea.MouseMsg:
		// Pointer movement never acknowledges a session's unseen state change.
		return model, nil
	case tea.KeyMsg:
		if model.projectSearching {
			return model.updateProjectSearch(message)
		}
		switch message.String() {
		case "q", "ctrl+c":
			model.cancelNavigation()
			return model, tea.Quit
		case "/":
			model.sidebarPreview = nil
			model.updateTableRows()
			model.projectSearching = true
			model.projectQuery = ""
			model.sidebarFocus = true
			model.sidebarCursor = model.projectItemStart()
			return model, nil
		case "p":
			if model.sidebarFocus {
				items := model.sidebarItems()
				if model.sidebarCursor < len(items) {
					item := items[model.sidebarCursor]
					if item.view == "" && item.provider == "" && !item.allTypes {
						if model.pinnedProjects == nil {
							model.pinnedProjects = make(map[string]bool)
						}
						model.pinnedProjects[item.project] = !model.pinnedProjects[item.project]
						if err := model.saveProjectPins(); err != nil {
							model.notice = safeDisplayText(fmt.Sprintf("Could not save project pins: %v", err))
							model.noticeRevision++
							return model, clearNotice(model.noticeRevision)
						}
						for index, candidate := range model.sidebarItems() {
							if candidate.project == item.project && candidate.view == "" && candidate.provider == "" && !candidate.allTypes {
								model.sidebarCursor = index
								break
							}
						}
					}
				}
			}
			return model, nil
		case "tab":
			model.sidebarFocus = !model.sidebarFocus
			model.sidebarPreview = nil
			if model.sidebarFocus {
				model.previewSidebarItem()
			}
			model.updateTableRows()
			return model, nil
		case "s":
			model.cycleSortColumn()
			return model, nil
		case "S":
			model.sort.descending = !model.sort.descending
			model.updateTableRows()
			return model, nil
		case "j", "down":
			if model.sidebarFocus {
				model.moveSidebarCursor(1)
				return model, nil
			}
		case "k", "up":
			if model.sidebarFocus {
				model.moveSidebarCursor(-1)
				return model, nil
			}
		case "enter":
			if model.sidebarFocus {
				items := model.sidebarItems()
				if model.sidebarCursor >= 0 && model.sidebarCursor < len(items) {
					item := items[model.sidebarCursor]
					if item.view != "" {
						if model.selectedView == item.view && model.selectedProject == "" {
							model.selectedView = allView
						} else {
							model.selectedView = item.view
						}
						model.selectedProject = ""
					} else if item.agentGroup {
						model.agentsExpanded = !model.agentsExpanded
						model.selectedProvider = ""
					} else if item.provider != "" {
						if model.selectedProvider == item.provider {
							model.selectedProvider = ""
						} else {
							model.selectedProvider = item.provider
						}
					}
					if item.view == "" && item.provider == "" && !item.allTypes {
						model.selectedProject = item.project
					}
					model.sidebarPreview = nil
					model.updateTableRows()
				}
				return model, nil
			}
			if !model.sidebarFocus {
				model.markSelectedSessionRead()
			}
			return model.navigateSelectedSession()
		}
	case polling.Update:
		model.applyProviderUpdate(message)
		if !slices.Contains(model.providers(), model.selectedProvider) {
			model.selectedProvider = ""
			model.updateTableRows()
		}
		model.sidebarCursor = min(model.sidebarCursor, len(model.sidebarItems())-1)
		return model, awaitProviderUpdate(model.updates)
	case sessionNavigationMessage:
		model.cancelNavigation()
		if message.err != nil {
			model.noticeRevision++
			model.notice = safeDisplayText(fmt.Sprintf("Could not open workspace: %v", message.err))
			return model, clearNotice(model.noticeRevision)
		}
		return model, tea.Quit
	case noticeExpiredMessage:
		if message.revision == model.noticeRevision {
			model.notice = ""
		}
		return model, nil
	}

	var command tea.Cmd
	model.table, command = model.table.Update(message)
	return model, command
}

func (model Model) selectedSession() (agent.Session, bool) {
	sessions := model.filteredSessions()
	selectedIndex := model.table.Cursor()
	if selectedIndex < 0 || selectedIndex >= len(sessions) {
		return agent.Session{}, false
	}

	return sessions[selectedIndex], true
}
