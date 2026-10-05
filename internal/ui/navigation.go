package ui

import (
	"context"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
)

// SessionNavigator opens the workspace that owns a selected session when the
// current terminal environment supports navigation.
type SessionNavigator interface {
	Navigate(context.Context, agent.Session) error
}

// ModelOption configures optional dashboard behavior.
type ModelOption func(*Model)

// WithSessionNavigator enables session navigation for terminal environments
// that can open a session's workspace.
func WithSessionNavigator(navigator SessionNavigator) ModelOption {
	return func(model *Model) {
		model.navigator = navigator
	}
}

// WithApplicationContext stops navigation when the application shuts down.
func WithApplicationContext(ctx context.Context) ModelOption {
	return func(model *Model) {
		model.appContext = ctx
	}
}

func (model Model) navigateSelectedSession() (tea.Model, tea.Cmd) {
	if model.sidebarFocus || model.navigator == nil || model.navigationCancel != nil {
		return model, nil
	}

	session, ok := model.selectedSession()
	if !ok {
		return model, nil
	}

	navigator := model.navigator
	requestContext, cancel := context.WithTimeout(model.appContext, requestTimeout)
	// Keep this request pending until its result is handled so repeated Enter
	// presses cannot start competing pane switches.
	model.navigationCancel = cancel
	return model, func() tea.Msg {
		defer cancel()
		if err := requestContext.Err(); err != nil {
			return sessionNavigationMessage{err: err}
		}

		return sessionNavigationMessage{err: navigator.Navigate(requestContext, session)}
	}
}

func (model *Model) cancelNavigation() {
	if model.navigationCancel != nil {
		model.navigationCancel()
		model.navigationCancel = nil
	}
}
