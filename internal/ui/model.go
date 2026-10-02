// Package ui renders the a-gent dashboard.
package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultTableWidth   = 70
	minimumTableWidth   = 34
	sidebarContentWidth = 18
	refreshInterval     = time.Second
	requestTimeout      = 2 * time.Second
	maximumSessionRows  = 6
)

var (
	appStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0A0A0A")).
			Padding(0, 1)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E6E6E6")).
			Background(lipgloss.Color("#161616")).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D0D0D0")).
			Background(lipgloss.Color("#0A0A0A")).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(1, 1, 0, 1).
			Width(sidebarContentWidth)
	panelStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0A0A0A")).
			Padding(0, 1)
	detailStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0A0A0A")).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(1, 1, 0, 1)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#969696"))
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#BDBDBD"))
	accentStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E6E6E6"))
	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3FB950"))
	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D29922"))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F85149"))
	footerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0A0A0A")).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Foreground(lipgloss.Color("#969696")).
			Padding(0, 1)
)

// Model holds the UI state for the application.
type Model struct {
	table     table.Model
	adapters  []agent.Adapter
	sessions  []agent.Session
	lastError error
	width     int
	height    int
}

type sessionsUpdatedMessage struct {
	sessions []agent.Session
	err      error
}

type refreshMessage time.Time

type sessionSummary struct {
	total   int
	running int
	waiting int
	idle    int
	errors  int
}

// NewModel creates the dashboard for the supplied provider adapters.
func NewModel(adapters []agent.Adapter) Model {
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

	return Model{table: agentTable, adapters: adapters}
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
		switch message.String() {
		case "q", "ctrl+c":
			return model, tea.Quit
		}
	case sessionsUpdatedMessage:
		if message.err == nil {
			model.sessions = message.sessions
		}
		model.lastError = message.err
		model.updateTableRows()
		return model, nil
	case refreshMessage:
		return model, tea.Batch(model.fetchSessions(), scheduleRefresh())
	}

	var command tea.Cmd
	model.table, command = model.table.Update(message)
	return model, command
}

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	summary := summarizeSessions(model.sessions)
	main := panelStyle.Width(model.table.Width()).Render(accentStyle.Render(model.sessionTitle()) + "\n" + model.sessionTableView())
	detail := detailStyle.Width(model.table.Width()).Render(model.detailView())
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, main, detail)
	sidebar := sidebarStyle.Height(lipgloss.Height(rightColumn)).Render(model.sidebarView(summary))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightColumn)

	headerText := lipgloss.JoinHorizontal(
		lipgloss.Left,
		accentStyle.Render("a-gent"),
		fmt.Sprintf("  /  %d sessions  /  %d running  /  live data", summary.total, summary.running),
	)
	contentWidth := lipgloss.Width(body)
	header := headerStyle.Width(contentWidth).Render(headerText)
	footer := footerStyle.Width(contentWidth).Render("j/k or ↑/↓: browse sessions  •  q: quit")

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
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

func (model Model) sidebarView(summary sessionSummary) string {
	lines := []string{
		sectionStyle.Render("OVERVIEW"),
		"",
		accentStyle.Render(fmt.Sprintf("Sessions     %d", summary.total)),
		runningStyle.Render(fmt.Sprintf("Running      %d", summary.running)),
		waitingStyle.Render(fmt.Sprintf("Waiting      %d", summary.waiting)),
		mutedStyle.Render(fmt.Sprintf("Idle         %d", summary.idle)),
		errorStyle.Render(fmt.Sprintf("Errors       %d", summary.errors)),
	}

	if model.lastError != nil {
		lines = append(lines, "", errorStyle.Render("● Unable to refresh"), mutedStyle.Render(model.lastError.Error()))
	} else {
		lines = append(lines, "", mutedStyle.Render("● Refreshes every 1s"))
	}

	return strings.Join(lines, "\n")
}

func (model Model) detailView() string {
	selectedIndex := model.table.Cursor()
	if selectedIndex < 0 || selectedIndex >= len(model.sessions) {
		if model.lastError != nil {
			return "SELECTED SESSION\nCould not read live sessions."
		}
		return "SELECTED SESSION\nNo live sessions found."
	}

	selectedSession := model.sessions[selectedIndex]
	return fmt.Sprintf(
		"%s\n%s  %s\n%s\nDirectory: %s\nSession: %s",
		sectionStyle.Render("SELECTED SESSION"),
		selectedSession.Provider,
		statusStyle(selectedSession.State).Render("● "+displayState(selectedSession.State)),
		selectedSession.Name,
		selectedSession.WorkingDirectory,
		selectedSession.ID,
	)
}

func (model Model) sessionTableView() string {
	columns := model.table.Columns()
	headerCells := make([]string, len(columns))
	for index, column := range columns {
		headerCells[index] = renderTableCell(column.Title, column.Width, sectionStyle, lipgloss.Color("#202020"))
	}

	rows := []string{lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)}
	if len(model.sessions) == 0 {
		rows = append(rows, mutedStyle.Render("No live sessions found."))
		return strings.Join(rows, "\n")
	}

	start, end := model.visibleSessionRange()
	for index := start; index < end; index++ {
		rows = append(rows, model.sessionRowView(index, columns))
	}

	return strings.Join(rows, "\n")
}

func (model Model) sessionTitle() string {
	if len(model.sessions) == 0 {
		return "SESSIONS"
	}

	start, end := model.visibleSessionRange()
	return fmt.Sprintf("SESSIONS (%d-%d of %d)", start+1, end, len(model.sessions))
}

func (model Model) visibleSessionRange() (int, int) {
	visibleRows := min(model.table.Height(), len(model.sessions))
	selectedIndex := model.table.Cursor()
	start := 0
	if selectedIndex >= visibleRows {
		start = selectedIndex - visibleRows + 1
	}

	end := min(start+visibleRows, len(model.sessions))
	return start, end
}

func (model Model) sessionRowView(index int, columns []table.Column) string {
	selected := index == model.table.Cursor()
	background := lipgloss.Color("")
	if selected {
		background = lipgloss.Color("#292929")
	}

	cells := make([]string, len(columns))
	for columnIndex, column := range columns {
		value, style := sessionColumnValue(model.sessions[index], column.Title)
		cells[columnIndex] = renderTableCell(value, column.Width, style, background)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func sessionColumnValue(session agent.Session, columnTitle string) (string, lipgloss.Style) {
	switch columnTitle {
	case "Agent":
		return session.Provider, lipgloss.NewStyle()
	case "Session":
		return session.Name, lipgloss.NewStyle()
	case "Directory":
		return filepath.Base(session.WorkingDirectory), mutedStyle
	case "Status":
		return displayState(session.State), statusStyle(session.State)
	default:
		return "", lipgloss.NewStyle()
	}
}

func renderTableCell(value string, width int, textStyle lipgloss.Style, background lipgloss.Color) string {
	cellStyle := textStyle.Width(width).MaxWidth(width).Padding(0, 1)
	if background != "" {
		cellStyle = cellStyle.Background(background)
	}

	return cellStyle.Render(value)
}

func statusStyle(state agent.State) lipgloss.Style {
	switch state {
	case agent.StateRunning:
		return runningStyle
	case agent.StateWaiting:
		return waitingStyle
	case agent.StateError, agent.StateUnavailable:
		return errorStyle
	default:
		return mutedStyle
	}
}

func displayState(state agent.State) string {
	if state == "" {
		return "Unavailable"
	}

	return strings.ToUpper(string(state[:1])) + string(state[1:])
}

func (model *Model) resizeTable() {
	tableWidth := defaultTableWidth
	if model.width > 0 {
		tableWidth = model.width - sidebarContentWidth - 6
	}
	tableWidth = max(tableWidth, minimumTableWidth)

	columns := tableColumns(tableWidth)
	model.table.SetWidth(tableWidth)
	model.table.SetColumns(columns)
	model.updateTableRows()

	tableHeight := min(maximumSessionRows, max(3, len(model.sessions)))
	if model.height > 0 {
		tableHeight = min(tableHeight, max(3, model.height-10))
	}
	model.table.SetHeight(tableHeight + 1)
}

func (model *Model) updateTableRows() {
	columns := model.table.Columns()
	rows := make([]table.Row, len(model.sessions))
	for index, session := range model.sessions {
		values := make([]string, len(columns))
		for columnIndex, column := range columns {
			values[columnIndex], _ = sessionColumnValue(session, column.Title)
		}
		rows[index] = table.Row(values)
	}

	model.table.SetRows(rows)
}

func tableColumns(tableWidth int) []table.Column {
	if tableWidth < 48 {
		return []table.Column{{Title: "Agent", Width: 12}, {Title: "Status", Width: tableWidth - 15}}
	}
	if tableWidth < 72 {
		return []table.Column{
			{Title: "Agent", Width: 12},
			{Title: "Session", Width: tableWidth - 29},
			{Title: "Status", Width: 12},
		}
	}

	return []table.Column{
		{Title: "Agent", Width: 12},
		{Title: "Session", Width: tableWidth / 3},
		{Title: "Directory", Width: tableWidth/3 - 3},
		{Title: "Status", Width: 12},
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
