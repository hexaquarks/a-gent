// Package ui renders the a-gent dashboard.
package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"a-gent/internal/agent"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

const (
	defaultTableWidth   = 70
	minimumTableWidth   = 34
	sidebarContentWidth = 22
	refreshInterval     = time.Second
	requestTimeout      = 2 * time.Second
	minimumSessionRows  = 3
	maximumSessionRows  = 8
	noticeDuration      = 3 * time.Second

	colorBackground = "#0D1117"
	colorMainText   = "#D7DEE8"
	colorSecondary  = "#8996AA"
	colorAccent     = "#69D3E7"
	colorSelection  = "#203949"
	colorAgent      = "#BB9AF7"
	colorRunning    = "#9ECE6A"
	colorAttention  = "#E0AF68"
	colorError      = "#F7768E"
	colorDivider    = "#293442"

	// popupChromeRows covers the header, table title and header, selected-session
	// panel, and footer around the reserved session rows.
	popupChromeRows = 15
	// popupBorderRows are drawn by tmux and are not available to the program.
	popupBorderRows = 2
	// PopupContentHeight is the terminal-row height needed by the dashboard.
	PopupContentHeight = maximumSessionRows + popupChromeRows
	// PopupHeight includes the tmux border so the dashboard header is never
	// clipped by the popup's interior height.
	PopupHeight = PopupContentHeight + popupBorderRows
)

var (
	appStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			Padding(0, 1)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorMainText)).
			Background(lipgloss.Color(colorBackground)).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMainText)).
			Background(lipgloss.Color(colorBackground)).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Padding(1, 1, 0, 1).
			Width(sidebarContentWidth)
	panelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMainText)).
			Background(lipgloss.Color(colorBackground)).
			Padding(0, 1)
	detailStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Padding(1, 1, 0, 1)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary))
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorMainText))
	accentStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorAccent))
	mainTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMainText))
	shortcutKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorAccent))
	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRunning))
	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAttention))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError))
	agentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAgent))
	footerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorBackground)).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDivider)).
			Foreground(lipgloss.Color(colorSecondary)).
			Padding(0, 1)
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
	project string
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

	model := Model{table: agentTable, adapters: adapters, selectedView: allView}
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
		switch message.String() {
		case "q", "ctrl+c":
			return model, tea.Quit
		case "tab":
			model.sidebarFocus = !model.sidebarFocus
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

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	summary := summarizeSessions(model.sessions)
	title := model.panelTitleStyle().Render(model.sessionTitle())
	main := panelStyle.Width(model.table.Width()).Render(title + "\n\n" + model.sessionTableView())
	detail := detailStyle.Width(model.table.Width()).Render(model.detailView())
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, main, detail)
	sidebar := sidebarStyle.Height(lipgloss.Height(rightColumn)).Render(model.renderSidebar(summary))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightColumn)

	headerText := lipgloss.JoinHorizontal(
		lipgloss.Left,
		accentStyle.Render("a-gent"),
		mutedStyle.Render(fmt.Sprintf("  /  %d sessions  /  ", summary.total)),
		runningStyle.Render(fmt.Sprintf("%d running", summary.running)),
		mutedStyle.Render("  /  live data"),
	)
	contentWidth := lipgloss.Width(body)
	header := headerStyle.Width(contentWidth).Render(headerText)
	footer := model.footerView(contentWidth)

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}

func (model Model) panelTitleStyle() lipgloss.Style {
	if model.sidebarFocus {
		return sectionStyle
	}
	return accentStyle
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

	lines = append(lines, "", projectsTitleStyle.Render("PROJECTS"))
	for index, item := range items[len(sidebarViews()):] {
		lines = append(lines, model.sidebarItemView(item, len(sidebarViews())+index, model.projectCount(item.project)))
	}

	if model.lastError != nil {
		lines = append(lines, "", errorStyle.Render("● Unable to refresh"))
	}

	return strings.Join(lines, "\n")
}

func (model Model) detailView() string {
	selectedSession, ok := model.selectedSession()
	if !ok {
		if model.lastError != nil {
			return model.panelTitleStyle().Render("SELECTED SESSION") + "\n" + errorStyle.Render("Could not read live sessions.")
		}
		return model.panelTitleStyle().Render("SELECTED SESSION") + "\n" + mutedStyle.Render("No live sessions found.")
	}

	return fmt.Sprintf(
		"%s\n%s  %s\n%s\n%s %s\n%s %s",
		model.panelTitleStyle().Render("SELECTED SESSION"),
		agentStyle.Render(selectedSession.Provider),
		statusStyle(selectedSession.State).Render("● "+displayState(selectedSession.State)),
		mainTextStyle.Bold(true).Render(selectedSession.Name),
		mutedStyle.Render("Directory:"),
		mainTextStyle.Render(selectedSession.WorkingDirectory),
		mutedStyle.Render("Session:"),
		mutedStyle.Render(selectedSession.ID),
	)
}

func (model Model) selectedSession() (agent.Session, bool) {
	sessions := model.filteredSessions()
	selectedIndex := model.table.Cursor()
	if selectedIndex < 0 || selectedIndex >= len(sessions) {
		return agent.Session{}, false
	}

	return sessions[selectedIndex], true
}

func (model Model) navigateSelectedSession() (tea.Model, tea.Cmd) {
	if model.sidebarFocus || model.navigator == nil {
		return model, nil
	}

	session, ok := model.selectedSession()
	if !ok {
		return model, nil
	}

	navigator := model.navigator
	return model, func() tea.Msg {
		requestContext, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		return sessionNavigationMessage{err: navigator.Navigate(requestContext, session)}
	}
}

func (model Model) footerText() string {
	parts := []string{
		shortcutKeyStyle.Render("tab") + mutedStyle.Render(": switch focus"),
		shortcutKeyStyle.Render("j/k or ↑/↓") + mutedStyle.Render(": browse"),
	}
	if model.navigator != nil {
		parts = append(parts, shortcutKeyStyle.Render("enter")+mutedStyle.Render(": open workspace"))
	}
	parts = append(parts, shortcutKeyStyle.Render("q")+mutedStyle.Render(": quit"))

	return strings.Join(parts, "  •  ")
}

func (model Model) footerView(width int) string {
	if model.notice == "" {
		return footerStyle.Width(width).Render(model.footerText())
	}

	messageWidth := max(0, width-4)
	message := runewidth.Truncate(model.notice, messageWidth, "…")
	return footerStyle.Width(width).Render(errorStyle.Render("! " + message))
}

// Error details can contain project paths. Keep terminal controls and newlines
// in those paths from executing or breaking the single-line notice layout.
func safeNoticeText(message string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, ansi.Strip(message))
}

func clearNotice(revision int) tea.Cmd {
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg {
		return noticeExpiredMessage{revision: revision}
	})
}

func (model Model) sessionTableView() string {
	columns := model.table.Columns()
	headerCells := make([]string, len(columns))
	for index, column := range columns {
		headerCells[index] = renderTableCell(column.Title, column.Width, sectionStyle, lipgloss.Color(colorDivider))
	}

	rows := []string{lipgloss.JoinHorizontal(lipgloss.Top, headerCells...), ""}
	start, end := model.visibleSessionRange()
	for index := start; index < end; index++ {
		rows = append(rows, model.sessionRowView(index, columns))
	}

	for placeholderIndex := end - start; placeholderIndex < model.table.Height(); placeholderIndex++ {
		rows = append(rows, model.emptySessionRowView(columns, placeholderIndex == 0))
	}

	return strings.Join(rows, "\n")
}

func (model Model) sessionTitle() string {
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return "SESSIONS"
	}

	start, end := model.visibleSessionRange()
	return fmt.Sprintf("SESSIONS (%d-%d of %d)", start+1, end, len(sessions))
}

func (model Model) visibleSessionRange() (int, int) {
	sessions := model.filteredSessions()
	if len(sessions) == 0 {
		return 0, 0
	}

	visibleRows := min(model.table.Height(), len(sessions))
	selectedIndex := model.table.Cursor()
	start := 0
	if selectedIndex >= visibleRows {
		start = selectedIndex - visibleRows + 1
	}

	end := min(start+visibleRows, len(model.filteredSessions()))
	return start, end
}

func (model Model) sessionRowView(index int, columns []table.Column) string {
	selected := index == model.table.Cursor()
	background := lipgloss.Color("")
	if selected {
		background = lipgloss.Color(colorSelection)
	}

	cells := make([]string, len(columns))
	for columnIndex, column := range columns {
		session := model.filteredSessions()[index]
		value, style := sessionColumnValue(session, column.Title)
		if selected && column.Title == "Agent" {
			value = "› " + value
		}
		cells[columnIndex] = renderTableCell(value, column.Width, style, background)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func (model Model) emptySessionRowView(columns []table.Column, showEmptyMessage bool) string {
	cells := make([]string, len(columns))
	emptyMessageColumn := 0
	emptyMessage := "No live"
	for columnIndex, column := range columns {
		if column.Title == "Session" {
			emptyMessageColumn = columnIndex
			emptyMessage = model.emptySessionMessage()
			break
		}
	}

	for columnIndex, column := range columns {
		value := ""
		style := lipgloss.NewStyle()
		if showEmptyMessage && columnIndex == emptyMessageColumn {
			value = emptyMessage
			style = mutedStyle
		}

		cells[columnIndex] = renderTableCell(value, column.Width, style, lipgloss.Color(""))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func (model Model) emptySessionMessage() string {
	if len(model.sessions) == 0 {
		return "No live sessions found."
	}

	return "No sessions match this filter."
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
	for _, project := range model.projects() {
		items = append(items, sidebarItem{label: projectName(project), project: project})
	}
	return items
}

func (model Model) sidebarItemView(item sidebarItem, index, count int) string {
	label := fmt.Sprintf("%-14s %d", item.label, count)
	selected := (item.view != "" && item.view == model.selectedView) ||
		(item.project != "" && item.project == model.selectedProject)
	focused := model.sidebarFocus && index == model.sidebarCursor

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
		if model.selectedProject != "" && session.WorkingDirectory != model.selectedProject {
			continue
		}
		if model.selectedProject == "" && !matchesView(session, model.selectedView) {
			continue
		}
		filteredSessions = append(filteredSessions, session)
	}
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

func sessionColumnValue(session agent.Session, columnTitle string) (string, lipgloss.Style) {
	switch columnTitle {
	case "Agent":
		return session.Provider, agentStyle
	case "Session":
		return session.Name, mainTextStyle
	case "Directory":
		return filepath.Base(session.WorkingDirectory), mutedStyle
	case "Status":
		return "● " + displayState(session.State), statusStyle(session.State)
	default:
		return "", lipgloss.NewStyle()
	}
}

func renderTableCell(value string, width int, textStyle lipgloss.Style, background lipgloss.Color) string {
	contentWidth := max(0, width-2)
	truncatedValue := runewidth.Truncate(value, contentWidth, "…")
	cellStyle := textStyle.Width(width).MaxWidth(width).Padding(0, 1)
	if background != "" {
		cellStyle = cellStyle.Background(background)
	}

	return cellStyle.Render(truncatedValue)
}

func statusStyle(state agent.State) lipgloss.Style {
	switch state {
	case agent.StateRunning:
		return runningStyle
	case agent.StateWaiting:
		return waitingStyle
	case agent.StateError:
		return errorStyle
	case agent.StateUnavailable:
		return waitingStyle
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

	tableHeight := maximumSessionRows
	if model.height > 0 {
		tableHeight = min(tableHeight, max(minimumSessionRows, model.height-10))
	}
	model.table.SetHeight(tableHeight + 1)
}

func (model *Model) updateTableRows() {
	columns := model.table.Columns()
	sessions := model.filteredSessions()
	rows := make([]table.Row, len(sessions))
	for index, session := range sessions {
		values := make([]string, len(columns))
		for columnIndex, column := range columns {
			values[columnIndex], _ = sessionColumnValue(session, column.Title)
		}
		rows[index] = table.Row(values)
	}

	model.table.SetRows(rows)
	if len(rows) == 0 {
		model.table.SetCursor(0)
		return
	}
	model.table.SetCursor(min(model.table.Cursor(), len(rows)-1))
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
