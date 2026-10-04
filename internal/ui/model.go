// Package ui renders the a-gent dashboard.
package ui

import (
	"context"
	"fmt"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	refreshInterval = time.Second
	requestTimeout  = 2 * time.Second
)

// Model holds the UI state for the application.
type Model struct {
	table           table.Model
	adapters        []agent.Adapter
	sessions        []agent.Session
	navigator       SessionNavigator
	selectedView    sidebarView
	selectedProject string
	sidebarFocus    bool
	sidebarCursor   int
	lastError       error
	notice          string
	noticeRevision  int
	width           int
	height          int
	sort            sessionSort
	sortMenuOpen    bool
	sortMenuCursor  int
	sortMenuDraft   sessionSort
	tableSessionIDs []sessionIdentity
}

type sessionsUpdatedMessage struct {
	sessions []agent.Session
	err      error
}

type refreshMessage time.Time

type sessionNavigationMessage struct {
	err error
}

type noticeExpiredMessage struct {
	revision int
}

// NewModel creates the dashboard for the supplied provider adapters.
func NewModel(adapters []agent.Adapter, options ...ModelOption) Model {
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
		table:        agentTable,
		adapters:     adapters,
		selectedView: allView,
		sort:         sessionSort{column: "Last active", descending: true},
	}
	for _, option := range options {
		option(&model)
	}

	return model
}

// Init starts the live provider refresh loop.
func (model Model) Init() tea.Cmd {
	return tea.Batch(model.fetchSessions(), scheduleRefresh())
}

// Update receives events and returns the next UI state for Bubble Tea to render.
func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		model.resizeTable()
		return model, nil
	case tea.KeyMsg:
		if model.sortMenuOpen {
			return model.updateSortMenu(message)
		}
		switch message.String() {
		case "q", "ctrl+c":
			return model, tea.Quit
		case "tab":
			model.sidebarFocus = !model.sidebarFocus
			return model, nil
		case "s":
			model.openSortMenu()
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
			return model.navigateSelectedSession()
		}
	case sessionsUpdatedMessage:
		if message.err == nil {
			model.sessions = message.sessions
			model.clearMissingProjectFilter()
		}
		model.lastError = message.err
		model.updateTableRows()
		return model, nil
	case refreshMessage:
		return model, tea.Batch(model.fetchSessions(), scheduleRefresh())
	case sessionNavigationMessage:
		if message.err != nil {
			model.noticeRevision++
			model.notice = safeNoticeText(fmt.Sprintf("Could not open workspace: %v", message.err))
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

func (model Model) fetchSessions() tea.Cmd {
	adapters := model.adapters
	return func() tea.Msg {
		requestContext, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		var sessions []agent.Session
		for _, adapter := range adapters {
			providerSessions, err := adapter.Sessions(requestContext)
			if err != nil {
				return sessionsUpdatedMessage{err: fmt.Errorf("read %s sessions: %w", adapter.Provider(), err)}
			}
			sessions = append(sessions, providerSessions...)
		}

		return sessionsUpdatedMessage{sessions: sessions}
	}
}

func scheduleRefresh() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return refreshMessage(time.Now())
	})
}

func (model Model) selectedSession() (agent.Session, bool) {
	sessions := model.filteredSessions()
	selectedIndex := model.table.Cursor()
	if selectedIndex < 0 || selectedIndex >= len(sessions) {
		return agent.Session{}, false
	}

	return sessions[selectedIndex], true
}
