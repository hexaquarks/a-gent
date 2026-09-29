package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
			Width(18)
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
	overviewValueStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#E6E6E6"))
	workingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3FB950"))
	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D29922"))
	footerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0A0A0A")).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Foreground(lipgloss.Color("#969696")).
			Padding(0, 1)
)

const (
	sidebarContentWidth = 18
	minimumTableWidth   = 34
	defaultTableWidth   = 70
)

// agent represents one mock coding agent shown in the interface.
type agent struct {
	agentType  string
	tmuxTarget string
	status     string
	age        string
	repository string
}

// agentSummary contains the counts displayed in the overview panel.
type agentSummary struct {
	total        int
	active       int
	waiting      int
	paused       int
	repositories int
}

// Model holds the UI state for the application.
type Model struct {
	table  table.Model
	agents []agent
	width  int
	height int
}

// NewModel creates the initial mock interface.
func NewModel() Model {
	agents := []agent{
		{agentType: "codex", tmuxTarget: "dev:1.2", status: "Working", age: "8m", repository: "a-gent"},
		{agentType: "claude-code", tmuxTarget: "api:3.1", status: "Working", age: "14m", repository: "api"},
		{agentType: "cursor", tmuxTarget: "web:2.3", status: "Waiting", age: "21m", repository: "website"},
		{agentType: "aider", tmuxTarget: "docs:1.1", status: "Running", age: "32m", repository: "docs"},
		{agentType: "opencode", tmuxTarget: "cli:4.2", status: "Working", age: "47m", repository: "cli"},
		{agentType: "goose", tmuxTarget: "mobile:1.3", status: "Waiting", age: "1h", repository: "mobile"},
		{agentType: "continue", tmuxTarget: "api:2.1", status: "Paused", age: "1h", repository: "api"},
		{agentType: "copilot", tmuxTarget: "web:4.1", status: "Running", age: "2h", repository: "website"},
		{agentType: "gemini-cli", tmuxTarget: "sync:2.2", status: "Working", age: "2h", repository: "sync"},
		{agentType: "roo-code", tmuxTarget: "cli:2.4", status: "Waiting", age: "3h", repository: "cli"},
		{agentType: "amp", tmuxTarget: "dev:3.2", status: "Working", age: "3h", repository: "a-gent"},
		{agentType: "devin", tmuxTarget: "docs:2.1", status: "Paused", age: "4h", repository: "docs"},
	}

	agentTable := table.New(
		table.WithColumns(tableColumns(defaultTableWidth)),
		table.WithRows(agentRows(agents, len(tableColumns(defaultTableWidth)))),
		table.WithFocused(true),
		table.WithHeight(7),
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

	return Model{table: agentTable, agents: agents}
}

// Init starts the Bubble Tea program with no initial command.
func (model Model) Init() tea.Cmd {
	return nil
}

// Update receives events and returns the next UI state for Bubble Tea to render.
func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if windowSize, ok := message.(tea.WindowSizeMsg); ok {
		model.width = windowSize.Width
		model.height = windowSize.Height
		model.resizeTable()
		return model, nil
	}

	if keyMessage, ok := message.(tea.KeyMsg); ok {
		switch keyMessage.String() {
		case "q", "ctrl+c":
			return model, tea.Quit
		}
	}

	var command tea.Cmd
	model.table, command = model.table.Update(message)
	return model, command
}

// View renders the current UI state after Bubble Tea calls Update.
func (model Model) View() string {
	summary := summarizeAgents(model.agents)
	main := panelStyle.Width(model.table.Width()).Render(accentStyle.Render("AGENTS") + "\n" + model.agentTableView())
	detail := detailStyle.Width(model.table.Width()).Render(model.detailView())
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, main, detail)
	sidebar := sidebarStyle.Height(lipgloss.Height(rightColumn)).Render(model.sidebarView(summary))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightColumn)

	headerText := lipgloss.JoinHorizontal(
		lipgloss.Left,
		accentStyle.Render("a-gent"),
		fmt.Sprintf("  /  %d agents  /  %d active  /  mock data", summary.total, summary.active),
	)
	contentWidth := lipgloss.Width(body)
	header := headerStyle.Width(contentWidth).Render(headerText)
	footer := footerStyle.Width(contentWidth).Render("j/k or ↑/↓: navigate  •  q: quit")

	dashboard := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))

	return dashboard
}

func (model Model) sidebarView(summary agentSummary) string {
	return strings.Join([]string{
		sectionStyle.Render("OVERVIEW"),
		"",
		accentStyle.Render(fmt.Sprintf("Agents       %d", summary.total)),
		workingStyle.Render(fmt.Sprintf("Active       %d", summary.active)),
		waitingStyle.Render(fmt.Sprintf("Waiting      %d", summary.waiting)),
		mutedStyle.Render(fmt.Sprintf("Paused       %d", summary.paused)),
		"",
		mutedStyle.Render(fmt.Sprintf("Tmux targets %d", summary.total)),
		mutedStyle.Render(fmt.Sprintf("Repositories %d", summary.repositories)),
		mutedStyle.Render("● Mock data"),
	}, "\n")
}

func (model Model) detailView() string {
	selectedIndex := model.table.Cursor()
	if selectedIndex < 0 || selectedIndex >= len(model.agents) {
		return "SELECTED AGENT\nNo agent selected"
	}

	selectedAgent := model.agents[selectedIndex]
	status := selectedAgent.status
	if status == "Working" || status == "Running" {
		status = workingStyle.Render("● " + status)
	} else {
		status = waitingStyle.Render("● " + status)
	}

	return fmt.Sprintf(
		"%s\n%s  %s  %s\nRepository: %s\nTmux: %s",
		sectionStyle.Render("SELECTED AGENT"),
		selectedAgent.agentType,
		status,
		mutedStyle.Render(selectedAgent.age),
		selectedAgent.repository,
		selectedAgent.tmuxTarget,
	)
}

func (model Model) agentTableView() string {
	columns := model.table.Columns()
	headerCells := make([]string, len(columns))

	for index, column := range columns {
		headerCells[index] = renderTableCell(column.Title, column.Width, sectionStyle, lipgloss.Color("#202020"))
	}

	rows := []string{lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)}
	start, end := model.visibleAgentRange()

	for index := start; index < end; index++ {
		rows = append(rows, model.agentRowView(index, columns))
	}

	return strings.Join(rows, "\n")
}

func (model Model) visibleAgentRange() (int, int) {
	visibleRows := model.table.Height()
	if visibleRows > len(model.agents) {
		visibleRows = len(model.agents)
	}

	selectedIndex := model.table.Cursor()
	start := 0
	if selectedIndex >= visibleRows {
		start = selectedIndex - visibleRows + 1
	}

	end := start + visibleRows
	if end > len(model.agents) {
		end = len(model.agents)
	}

	return start, end
}

func (model Model) agentRowView(index int, columns []table.Column) string {
	selected := index == model.table.Cursor()
	background := lipgloss.Color("")
	if selected {
		background = lipgloss.Color("#292929")
	}

	cells := make([]string, len(columns))
	for columnIndex, column := range columns {
		value, style := agentColumnValue(model.agents[index], column.Title)
		cells[columnIndex] = renderTableCell(value, column.Width, style, background)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func agentColumnValue(agent agent, columnTitle string) (string, lipgloss.Style) {
	switch columnTitle {
	case "Agent":
		return agent.agentType, lipgloss.NewStyle()
	case "Tmux":
		return agent.tmuxTarget, mutedStyle
	case "Repo":
		return agent.repository, lipgloss.NewStyle()
	case "Status":
		return agent.status, statusStyle(agent.status)
	case "Age":
		return agent.age, ageStyle(agent.age)
	default:
		return "", lipgloss.NewStyle()
	}
}

func renderTableCell(value string, width int, textStyle lipgloss.Style, background lipgloss.Color) string {
	cellStyle := textStyle.
		Width(width).
		MaxWidth(width).
		Padding(0, 1)

	if background != "" {
		cellStyle = cellStyle.Background(background)
	}

	return cellStyle.Render(value)
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "Working", "Running":
		return workingStyle
	case "Waiting":
		return waitingStyle
	default:
		return mutedStyle
	}
}

func ageStyle(age string) lipgloss.Style {
	if strings.HasSuffix(age, "h") {
		return waitingStyle
	}

	return mutedStyle
}

func (model *Model) resizeTable() {
	tableWidth := defaultTableWidth
	if model.width > 0 {
		tableWidth = model.width - sidebarContentWidth - 6
	}

	if tableWidth < minimumTableWidth {
		tableWidth = minimumTableWidth
	}

	columns := tableColumns(tableWidth)
	model.table.SetWidth(tableWidth)
	model.table.SetRows(agentRows(model.agents, len(columns)))
	model.table.SetColumns(columns)

	tableHeight := len(model.agents) + 2
	if model.height > 0 {
		availableHeight := model.height - 10
		if tableHeight > availableHeight {
			tableHeight = availableHeight
		}
	}

	if tableHeight < 5 {
		tableHeight = 5
	}

	model.table.SetHeight(tableHeight)
}

func tableColumns(tableWidth int) []table.Column {
	if tableWidth < 42 {
		return []table.Column{
			{Title: "Agent", Width: 14},
			{Title: "Tmux", Width: tableWidth - 17},
		}
	}

	if tableWidth < 56 {
		return []table.Column{
			{Title: "Agent", Width: 14},
			{Title: "Tmux", Width: tableWidth - 29},
			{Title: "Age", Width: 6},
		}
	}

	if tableWidth < 82 {
		return []table.Column{
			{Title: "Agent", Width: 16},
			{Title: "Tmux", Width: tableWidth - 58},
			{Title: "Repo", Width: 14},
			{Title: "Status", Width: 10},
			{Title: "Age", Width: 6},
		}
	}

	contentWidth := tableWidth - 10
	agentWidth := contentWidth / 5
	tmuxWidth := contentWidth * 35 / 100
	repositoryWidth := contentWidth / 5
	statusWidth := contentWidth * 15 / 100
	ageWidth := contentWidth - agentWidth - tmuxWidth - repositoryWidth - statusWidth

	return []table.Column{
		{Title: "Agent", Width: agentWidth},
		{Title: "Tmux", Width: tmuxWidth},
		{Title: "Repo", Width: repositoryWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Age", Width: ageWidth},
	}
}

func agentRows(agents []agent, columnCount int) []table.Row {
	rows := make([]table.Row, len(agents))

	for index, agent := range agents {
		values := []string{agent.agentType, agent.tmuxTarget, agent.repository, agent.status, agent.age}
		rows[index] = table.Row(values[:columnCount])
	}

	return rows
}

func summarizeAgents(agents []agent) agentSummary {
	repositories := make(map[string]struct{})
	summary := agentSummary{total: len(agents)}

	for _, agent := range agents {
		repositories[agent.repository] = struct{}{}

		switch agent.status {
		case "Working", "Running":
			summary.active++
		case "Waiting":
			summary.waiting++
		case "Paused":
			summary.paused++
		}
	}

	summary.repositories = len(repositories)
	return summary
}
