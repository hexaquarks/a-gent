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
	table            table.Model
	updates          <-chan polling.Update
	providerErrors   map[string]error
	sessions         []agent.Session
	navigator        SessionNavigator
	selectedView     sidebarView
	selectedProject  string
	selectedProvider string
	agentsExpanded   bool
	sidebarFocus     bool
	sidebarCursor    int
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
	return awaitProviderUpdate(model.updates)
}

// Update receives events and returns the next UI state for Bubble Tea to render.
func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
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
		switch message.String() {
		case "q", "ctrl+c":
			model.cancelNavigation()
			return model, tea.Quit
		case "tab":
			model.sidebarFocus = !model.sidebarFocus
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
				if model.sidebarCursor < len(items) && items[model.sidebarCursor].agentGroup {
					model.agentsExpanded = !model.agentsExpanded
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
