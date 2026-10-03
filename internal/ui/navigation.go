package ui

import (
	"context"

	"a-gent/internal/agent"
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
