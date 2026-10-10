package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// AgentLauncher starts an interactive coding agent in a project directory.
type AgentLauncher interface {
	Launch(ctx context.Context, provider, directory string, newWindow bool) error
}

// WithAgentLauncher enables starting agents from the dashboard.
func WithAgentLauncher(launcher AgentLauncher) ModelOption {
	return func(model *Model) { model.launcher = launcher }
}

type newAgentDialog struct {
	directory string
	provider  string
	err       error
	starting  bool
	newWindow bool
}

type agentLaunchMessage struct{ err error }

func (model Model) openNewAgent(newWindow bool) (tea.Model, tea.Cmd) {
	directory := model.selectedProject
	if model.sidebarFocus {
		items := model.sidebarItems()
		if model.sidebarCursor >= 0 && model.sidebarCursor < len(items) && items[model.sidebarCursor].project != "" {
			directory = items[model.sidebarCursor].project
		}
	} else if session, ok := model.selectedSession(); ok {
		directory = session.WorkingDirectory
	}
	if directory == "" {
		directory, _ = os.Getwd()
	}
	model.newAgent = &newAgentDialog{directory: directory, provider: "codex", newWindow: newWindow}
	return model, nil
}

func (model Model) updateNewAgent(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Copy dialog state because Bubble Tea models are passed by value.
	dialog := *model.newAgent
	model.newAgent = &dialog
	if key.String() == "ctrl+c" {
		model.cancelLaunch()
		return model, tea.Quit
	}
	if dialog.starting {
		return model, nil
	}
	switch key.String() {
	case "esc", "q":
		model.newAgent = nil
	case "left", "right", "tab", "h", "l":
		if dialog.provider == "codex" {
			dialog.provider = "claude"
		} else {
			dialog.provider = "codex"
		}
		dialog.err = nil
	case "enter":
		if model.launcher == nil {
			dialog.err = fmt.Errorf("Open a-gent inside tmux to start an agent")
			return model, nil
		}
		ctx, cancel := context.WithTimeout(model.appContext, requestTimeout)
		model.launchCancel = cancel
		dialog.starting = true
		dialog.err = nil
		launcher := model.launcher
		return model, func() tea.Msg {
			defer cancel()
			return agentLaunchMessage{err: launcher.Launch(ctx, dialog.provider, dialog.directory, dialog.newWindow)}
		}
	}
	return model, nil
}

func (model *Model) cancelLaunch() {
	if model.launchCancel != nil {
		model.launchCancel()
		model.launchCancel = nil
	}
}

// Overlay using terminal column offsets so styled and wide text stays aligned.
func (model Model) newAgentView(background string) string {
	dialog := model.newAgent
	width := min(54, max(1, model.width-4))
	contentWidth := max(1, width-6)
	field := func(label, value string) string {
		return mutedStyle.Render(fmt.Sprintf("%-11s", label)) + mainTextStyle.Render(ansi.Truncate(safeDisplayText(value), max(1, contentWidth-11), "…"))
	}
	directory := dialog.directory
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(directory, home+string(os.PathSeparator)) {
		directory = "~" + strings.TrimPrefix(directory, home)
	}
	providers := make([]string, 0, 2)
	for _, provider := range []string{"codex", "claude"} {
		style := providerStyle(provider).Padding(0, 1)
		if provider == dialog.provider {
			style = style.Foreground(lipgloss.Color(colorBackground)).Background(lipgloss.Color(colorAccent)).Bold(true)
		}
		providers = append(providers, style.Render(provider))
	}
	destination := "pane"
	if dialog.newWindow {
		destination = "window"
	}
	lines := []string{
		accentStyle.Render("NEW AGENT"), "",
		field("Project", projectName(dialog.directory)), field("Directory", directory), "",
		mutedStyle.Render("Provider   ") + strings.Join(providers, "  "),
		mutedStyle.Render("           ←/→ choose provider"), "",
		mainTextStyle.Render("Open in a new tmux " + destination), "",
		accentStyle.Render("Enter") + mutedStyle.Render(" start    ") + accentStyle.Render("Esc/q") + mutedStyle.Render(" cancel"),
	}
	if dialog.starting {
		lines[len(lines)-1] = accentStyle.Render("Starting agent…")
	}
	if dialog.err != nil {
		lines = append(lines, "", errorStyle.Render(safeDisplayText(dialog.err.Error())))
	}
	content := lipgloss.NewStyle().Width(contentWidth).Render(strings.Join(lines, "\n"))
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(colorAccent)).Background(lipgloss.Color(colorBackground)).Padding(1, 2).Render(content)
	return model.overlayPanel(background, box)
}
