package ui

import (
	"fmt"
	"strings"
	"testing"

	"a-gent/internal/agent"
	"a-gent/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpOwnsKeysAndClosesTopPanel(t *testing.T) {
	for _, underlying := range []string{"dashboard", "preview", "new agent"} {
		for _, closeKey := range []tea.KeyMsg{{Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune("q")}} {
			t.Run(fmt.Sprint(underlying, closeKey.String()), func(t *testing.T) {
				model := referenceDashboard()
				model.previewExpanded = underlying == "preview"
				if underlying == "new agent" {
					model.newAgent = &newAgentDialog{provider: "codex", directory: "/project"}
				}
				model = sendProjectKey(model, runeKey('?'))
				if !model.helpOpen || !strings.Contains(model.View(), "KEYBOARD HELP") {
					t.Fatal("help did not open")
				}
				cursor, sort := model.table.Cursor(), model.sort
				for _, key := range []tea.KeyMsg{runeKey('n'), runeKey('s'), runeKey('v'), {Type: tea.KeyTab}, {Type: tea.KeyEnter}} {
					model = sendProjectKey(model, key)
				}
				if model.table.Cursor() != cursor || model.sort != sort || model.sidebarFocus {
					t.Fatal("help leaked keys to dashboard")
				}
				if (model.newAgent != nil) != (underlying == "new agent") || model.previewExpanded != (underlying == "preview") {
					t.Fatal("help changed underlying panel")
				}
				// Live polling continues while help is open.
				updated, _ := model.Update(polling.Update{Provider: "extra", Sessions: []agent.Session{{ID: "new", State: agent.StateRunning}}})
				model = updated.(Model)
				if len(model.sessions) != 7 || !model.helpOpen {
					t.Fatal("help blocked live updates")
				}
				updated, command := model.Update(closeKey)
				model = updated.(Model)
				if command != nil || model.helpOpen {
					t.Fatal("close key did not dismiss just help")
				}
				if model.previewExpanded != (underlying == "preview") || (model.newAgent != nil) != (underlying == "new agent") {
					t.Fatal("closing help closed underlying panel")
				}
			})
		}
	}
}

func TestHelpSearchInputAndQuit(t *testing.T) {
	model := referenceDashboard()
	model = sendProjectKey(model, runeKey('/'))
	model = sendProjectKey(model, runeKey('?'))
	if model.helpOpen || model.projectQuery != "?" {
		t.Fatal("help intercepted search text")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	model = sendProjectKey(model, runeKey('?'))
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if command == nil || command() != tea.Quit() {
		t.Fatal("Ctrl+C did not quit help")
	}
}

func TestHelpFitsAndScrolls(t *testing.T) {
	for _, size := range [][2]int{{120, 28}, {80, 24}, {40, 16}, {24, 10}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			model := referenceDashboard()
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model = sendProjectKey(updated.(Model), runeKey('?'))
			for _, key := range []tea.KeyMsg{{Type: tea.KeyHome}, {Type: tea.KeyPgDown}, {Type: tea.KeyEnd}, {Type: tea.KeyPgUp}} {
				model = sendProjectKey(model, key)
				view := model.View()
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("panel exceeds terminal:\n%s", view)
				}
				if !strings.Contains(ansi.Strip(view), "Esc/q") {
					t.Fatalf("close hint missing:\n%s", view)
				}
			}
			width, visibleLines := model.helpDimensions()
			if len(helpLines(width)) <= visibleLines && model.helpScroll != 0 {
				t.Fatal("help scrolled despite fitting the panel")
			}
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnd})
			if !strings.Contains(ansi.Strip(model.View()), "Quit dashboard") {
				t.Fatal("last help action is inaccessible")
			}
			model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyHome})
			if model.helpScroll != 0 {
				t.Fatal("Home did not return to top")
			}
		})
	}
}
