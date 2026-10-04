// Package codex reads live sessions from Codex's local app-server daemon.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"a-gent/internal/agent"
)

const providerName = "codex"

// Adapter reads the sessions currently loaded by the local Codex daemon.
type Adapter struct{}

// NewAdapter creates a read-only Codex session adapter.
func NewAdapter() Adapter {
	return Adapter{}
}

// Provider returns the name of the provider this adapter supports.
func (Adapter) Provider() string {
	return providerName
}

// Sessions returns loaded Codex conversations, excluding workers and empty idle threads.
func (Adapter) Sessions(context context.Context) ([]agent.Session, error) {
	socketPath, err := daemonSocketPath(context)
	if err != nil {
		return nil, err
	}

	client, err := connect(context, socketPath)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.initialize(); err != nil {
		return nil, err
	}

	threadIDs, err := client.loadedThreadIDs()
	if err != nil {
		return nil, err
	}

	sessions := make([]agent.Session, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		thread, err := client.thread(threadID)
		if err != nil {
			return nil, err
		}

		if !thread.isVisibleSession() {
			continue
		}

		sessions = append(sessions, thread.session())
	}

	return sessions, nil
}

func (thread thread) isVisibleSession() bool {
	// Workers share their parent's directory but have no separate workspace.
	if thread.isSubagent() || thread.Status.Type == "notLoaded" {
		return false
	}

	// Loaded threads can include unused chat drafts. Codex synthesizes their
	// timestamps on each read, making them look perpetually recently active.
	// Keep them once they have content or start running; retain errors and
	// unfamiliar statuses so diagnostics are not silently hidden.
	return thread.Status.Type != "idle" || thread.hasConversationContent()
}

func (thread thread) hasConversationContent() bool {
	return strings.TrimSpace(thread.Name) != "" || strings.TrimSpace(thread.Preview) != ""
}

func (thread thread) session() agent.Session {
	var lastActiveAt time.Time
	// Empty threads may carry synthesized timestamps rather than real activity.
	if thread.UpdatedAt > 0 && thread.hasConversationContent() {
		lastActiveAt = time.Unix(thread.UpdatedAt, 0)
	}

	return agent.Session{
		ID:               thread.ID,
		Provider:         providerName,
		Name:             thread.Name,
		Preview:          thread.Preview,
		WorkingDirectory: thread.WorkingDirectory,
		State:            stateFromStatus(thread.Status),
		LastActiveAt:     lastActiveAt,
	}
}

func (thread thread) isSubagent() bool {
	if thread.ParentThreadID != "" {
		return true
	}

	// Top-level sources are strings (cli, vscode, etc.); worker sources are
	// tagged objects. Keep unfamiliar top-level sources visible.
	var source struct {
		SubAgent json.RawMessage `json:"subAgent"`
	}
	if err := json.Unmarshal(thread.Source, &source); err != nil {
		return false
	}

	return len(source.SubAgent) > 0 && string(source.SubAgent) != "null"
}

type daemonVersion struct {
	SocketPath string `json:"socketPath"`
	Status     string `json:"status"`
}

func daemonSocketPath(context context.Context) (string, error) {
	command := exec.CommandContext(context, "codex", "app-server", "daemon", "version")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read Codex daemon details: %w", err)
	}

	var version daemonVersion
	if err := json.Unmarshal(output, &version); err != nil {
		return "", fmt.Errorf("decode Codex daemon details: %w", err)
	}

	if version.Status != "running" || version.SocketPath == "" {
		return "", fmt.Errorf("Codex daemon is not running")
	}

	return version.SocketPath, nil
}

type threadStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags"`
}

func stateFromStatus(status threadStatus) agent.State {
	switch status.Type {
	case "active":
		for _, activeFlag := range status.ActiveFlags {
			if activeFlag == "waitingOnApproval" {
				return agent.StateWaiting
			}
		}
		return agent.StateRunning
	case "idle":
		return agent.StateIdle
	case "systemError":
		return agent.StateError
	default:
		return agent.StateUnavailable
	}
}
