// Package agent defines provider-neutral coding-agent session data.
package agent

import "context"

// State describes the current activity of one coding-agent session.
type State string

const (
	// StateRunning means the agent is processing a request.
	StateRunning State = "running"
	// StateWaiting means the agent needs a decision or additional input.
	StateWaiting State = "waiting"
	// StateIdle means the agent is ready for another request.
	StateIdle State = "idle"
	// StateError means the provider reported a session error.
	StateError State = "error"
	// StateUnavailable means a-gent cannot read the session's live state.
	StateUnavailable State = "unavailable"
)

// Session is a provider-neutral view of one coding-agent session.
type Session struct {
	ID               string
	Provider         string
	Name             string
	Preview          string
	WorkingDirectory string
	State            State
}

// Adapter reads live sessions from one coding-agent provider.
type Adapter interface {
	Provider() string
	Sessions(context.Context) ([]Session, error)
}
