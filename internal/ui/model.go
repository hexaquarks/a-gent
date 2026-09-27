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
			Padding(1, 2)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1).
			Width(20)
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	detailStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
)

// agent represents one mock coding agent shown in the interface.
type agent struct {
	name     string
	status   string
	project  string
	activity string
}

// Model holds the UI state for the application.
type Model struct {
	table  table.Model
	agents []agent
}

// NewModel creates the initial mock interface.
func NewModel() Model {
	agents := []agent{
		{name: "codex", status: "Working", project: "a-gent", activity: "Building the initial TUI"},
		{name: "claude-code", status: "Waiting", project: "website", activity: "Waiting for your input"},
		{name: "cursor", status: "Running", project: "api", activity: "Reviewing route handlers"},
	}

	rows := make([]table.Row, len(agents))
	for index, agent := range agents {
		rows[index] = table.Row{agent.name, agent.status, agent.project}
	}

	columns := []table.Column{
		{Title: "Agent", Width: 16},
		{Title: "Status", Width: 12},
		{Title: "Project", Width: 20},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(lipgloss.Color("62"))
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	t.SetStyles(styles)

	return Model{table: t, agents: agents}
}

// Init starts the Bubble Tea program with no initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles keyboard input and forwards navigation to the table.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyMessage, ok := message.(tea.KeyMsg); ok {
		switch keyMessage.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	var command tea.Cmd
	m.table, command = m.table.Update(message)
	return m, command
}

// View renders the mock dashboard.
func (m Model) View() string {
	header := headerStyle.Render("a-gent  •  AI coding agent monitor")
	sidebar := sidebarStyle.Render(strings.Join([]string{
		"VIEWS",
		"",
		"› Agents",
		"  Activity",
		"  Settings",
	}, "\\n"))

	main := panelStyle.Render("AGENTS\\n\\n" + m.table.View())
	top := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, "  ", main)

	detail := m.detailView()
	footer := mutedStyle.Render("j/k or ↑/↓: navigate  •  q: quit")

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", top, "", detail, "", footer))
}

func (m Model) detailView() string {
	selectedIndex := m.table.Cursor()
	if selectedIndex < 0 || selectedIndex >= len(m.agents) {
		return detailStyle.Render("DETAILS\\nNo agent selected")
	}

	selectedAgent := m.agents[selectedIndex]
	content := fmt.Sprintf(
		"DETAILS\\n%s  •  %s\\n%s\\n%s",
		selectedAgent.name,
		selectedAgent.status,
		selectedAgent.project,
		selectedAgent.activity,
	)

	return detailStyle.Render(content)
}
