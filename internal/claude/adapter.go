// Package claude lists active Claude Code sessions using its command-line tool.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/editpreview"
)

type commandRunner func(context.Context) ([]byte, error)

// Adapter reads Claude's sessions and converts them to the format shared by providers.
type Adapter struct {
	run   commandRunner
	edits *editpreview.Reader
}

func NewAdapter() Adapter {
	return Adapter{edits: &editpreview.Reader{}, run: func(ctx context.Context) ([]byte, error) {
		command := exec.CommandContext(ctx, "claude", "agents", "--json")
		command.WaitDelay = 250 * time.Millisecond
		return command.Output()
	}}
}

func (Adapter) Provider() string { return "claude" }

func (adapter Adapter) Sessions(ctx context.Context) ([]agent.Session, error) {
	output, err := adapter.run(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Claude Code sessions (requires claude agents --json): %w", err)
	}
	return decodeSessions(output)
}

type session struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	PID       int    `json:"pid"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Directory string `json:"cwd"`
	Status    string `json:"status"`
	State     string `json:"state"`
	StartedAt int64  `json:"startedAt"`
}

// Converts Claude's active sessions to the format the dashboard uses.
// Keeps unfinished background jobs even when their process has exited.
func decodeSessions(output []byte) ([]agent.Session, error) {
	var entries []session
	// Invalid output must report an error rather than make sessions disappear.
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "[") {
		return nil, fmt.Errorf("Claude Code returned an invalid session list")
	}
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("decode Claude Code sessions: %w", err)
	}
	sessions := make([]agent.Session, 0, len(entries))
	for _, entry := range entries {
		// Show user sessions, leaving out their internal helper agents.
		if entry.Kind != "interactive" && entry.Kind != "background" {
			continue
		}
		if entry.PID <= 0 && entry.State != "working" && entry.State != "blocked" {
			continue
		}

		id := entry.SessionID
		if entry.Kind == "background" && entry.ID != "" {
			// Keep the same session ID when a background job starts or restarts.
			id = "job:" + entry.ID
		}
		if id == "" && entry.PID > 0 {
			id = "process:" + strconv.Itoa(entry.PID) + ":" + strconv.FormatInt(entry.StartedAt, 10)
		}
		if id == "" {
			return nil, fmt.Errorf("Claude Code returned a session without an identity")
		}

		sessions = append(sessions, agent.Session{
			ID:               id,
			TranscriptID:     entry.SessionID,
			Provider:         "claude",
			Name:             entry.Name,
			WorkingDirectory: entry.Directory,
			ProcessID:        &entry.PID,
			State:            entry.activity(),
			// Claude reports when it started, but not when it last did work.
		})
	}
	return sessions, nil
}

// Uses the background job's status because its work can continue between processes.
// A permission prompt still means the user needs to act, even on a working job.
func (entry session) activity() agent.State {
	switch entry.State {
	case "working":
		if entry.Status == "waiting" {
			return agent.StateWaiting
		}
		return agent.StateRunning
	case "blocked":
		return agent.StateWaiting
	case "failed":
		return agent.StateError
	case "done", "stopped":
		return agent.StateIdle
	}
	switch entry.Status {
	case "busy":
		return agent.StateRunning
	case "waiting":
		return agent.StateWaiting
	case "idle":
		return agent.StateIdle
	default:
		return agent.StateUnavailable
	}
}
