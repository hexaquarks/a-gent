// Package agent defines provider-neutral coding-agent session data.
package agent

import (
	"context"
	"time"
)

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
	// TranscriptPath and TranscriptID identify provider-owned edit records.
	TranscriptPath   string
	TranscriptID     string
	ID               string
	Provider         string
	Name             string
	Preview          string
	WorkingDirectory string
	State            State

	// ProcessID identifies the process whose tmux pane should open.
	// Nil means match by directory; zero or less means there is no running process.
	ProcessID *int

	// Stale means these sessions were kept from a previous successful poll.
	Stale bool

	// LastActiveAt is the provider's last session update; zero means unknown.
	LastActiveAt time.Time
}

// Adapter reads live sessions from one coding-agent provider.
// Sessions must stop when its context is cancelled and return data the caller
// can change without affecting the adapter's own data.
type Adapter interface {
	Provider() string
	Sessions(context.Context) ([]Session, error)
}
