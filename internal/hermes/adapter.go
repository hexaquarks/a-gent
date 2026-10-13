// Package hermes observes live Hermes processes and their native plugin lifecycle.
package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"a-gent/internal/agent"
)

// Adapter reads metadata published by the bundled Hermes observer plugin.
type Adapter struct {
	discover  func(context.Context) ([]instance, error)
	directory string
	now       func() time.Time
	setupErr  error
}

// NewAdapter discovers Hermes without reading saved sessions as live instances.
func NewAdapter() Adapter {
	directory, _ := observerDirectory()
	return Adapter{discover: discoverInstances, directory: directory, now: time.Now}
}

// EnableObserver configures normal Hermes launches to publish native live status.
// Sessions reports setup failures through the normal provider error path.
func (adapter *Adapter) EnableObserver() {
	if _, err := exec.LookPath("hermes"); err != nil {
		return
	}
	_, adapter.setupErr = ConfigureObserver()
}

// Provider returns the provider name used by launching, navigation, and previews.
func (Adapter) Provider() string { return "hermes" }

type runtimeRecord struct {
	Version      int    `json:"version"`
	PID          int    `json:"pid"`
	StartedAt    int64  `json:"startedAt"`
	UpdatedAt    int64  `json:"updatedAt"`
	LastActiveAt int64  `json:"lastActiveAt"`
	Directory    string `json:"directory"`
	SessionID    string `json:"sessionID"`

	Name  string      `json:"name"`
	State agent.State `json:"state"`

	ParentID string          `json:"parentID"`
	Children []runtimeRecord `json:"children"`
}

func (adapter Adapter) runtime(instance instance) (runtimeRecord, error) {
	var record runtimeRecord
	if adapter.directory == "" {
		return record, fmt.Errorf("Hermes observer directory unavailable")
	}
	path := filepath.Join(adapter.directory, "instances", strconv.Itoa(instance.pid)+".json")
	file, err := os.Open(path)
	if err != nil {
		return record, fmt.Errorf("Hermes observer is not loaded; restart Hermes to load the a-gent plugin")
	}
	defer file.Close()
	if err := json.NewDecoder(io.LimitReader(file, 64*1024)).Decode(&record); err != nil {
		return record, fmt.Errorf("decode Hermes observer: %w", err)
	}
	if !record.matches(instance) {
		return record, fmt.Errorf("Hermes observer metadata does not match the live process")
	}
	age := adapter.now().Sub(time.UnixMilli(record.UpdatedAt))
	if age > 5*time.Second || age < -2*time.Second {
		return record, fmt.Errorf("Hermes observer metadata is stale")
	}
	return record, nil
}

// ps rounds process start times to seconds. A matching birth time keeps a reused
// PID from inheriting stale metadata while preserving identity across read errors.
func (record runtimeRecord) matches(instance instance) bool {
	difference := time.UnixMilli(record.StartedAt).Sub(instance.started)
	return record.Version == 1 && record.PID == instance.pid && difference >= -2*time.Second &&
		difference <= 2*time.Second && record.SessionID != "" && filepath.IsAbs(record.Directory)
}

// Sessions reports only live processes. Persisted observer files and Hermes's saved
// history never create dashboard rows without a corresponding live process.
func (adapter Adapter) Sessions(ctx context.Context) ([]agent.Session, error) {
	if adapter.setupErr != nil {
		return nil, adapter.setupErr
	}
	instances, err := adapter.discover(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]agent.Session, 0, len(instances))
	for _, instance := range instances {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := agent.Session{ID: instance.identity(), Provider: "hermes", Name: "Hermes",
			ProcessID: &instance.pid, State: agent.StateUnavailable, WorkingDirectory: instance.directory}
		record, err := adapter.runtime(instance)
		if err != nil {
			entry.Preview = err.Error()
		}
		if record.matches(instance) {
			entry.ID += ":" + record.SessionID
			entry.Name = record.Name
			entry.WorkingDirectory = record.Directory
			entry.TranscriptID = record.SessionID
			entry.LastActiveAt = time.UnixMilli(record.LastActiveAt)
			validState := record.State == agent.StateRunning || record.State == agent.StateIdle ||
				record.State == agent.StateWaiting
			if err == nil && validState {
				entry.State = record.State
			}
		}
		sessions = append(sessions, entry)
		if err != nil {
			continue
		}

		// Children share the validated live process, but have distinct session
		// identities and previews. Only trees rooted in this session are live.
		children := make(map[string]runtimeRecord)
		for _, child := range record.Children {
			validState := child.State == agent.StateRunning || child.State == agent.StateIdle ||
				child.State == agent.StateWaiting || child.State == agent.StateError
			if !safeSessionID(child.SessionID) || child.ParentID == "" || child.SessionID == record.SessionID ||
				child.Directory != record.Directory || !validState {
				continue
			}
			children[child.SessionID] = child
		}
		for _, child := range children {
			parent := child.ParentID
			visited := map[string]bool{child.SessionID: true}
			for parent != record.SessionID && !visited[parent] {
				visited[parent] = true
				ancestor, exists := children[parent]
				if !exists {
					break
				}
				parent = ancestor.ParentID
			}
			if parent != record.SessionID {
				continue
			}
			var lastActiveAt time.Time
			if child.LastActiveAt > 0 {
				lastActiveAt = time.UnixMilli(child.LastActiveAt)
			}

			sessions = append(sessions, agent.Session{
				ID:       instance.identity() + ":" + child.SessionID,
				ParentID: instance.identity() + ":" + child.ParentID,
				Provider: "hermes", Name: child.Name, ProcessID: &instance.pid,
				TranscriptID: child.SessionID, WorkingDirectory: child.Directory,
				State: child.State, LastActiveAt: lastActiveAt,
			})
		}
	}
	return sessions, nil
}

// safeSessionID keeps native child IDs inside a single preview filename.
func safeSessionID(sessionID string) bool {
	return sessionID != "" && sessionID != "." && sessionID != ".." &&
		!strings.ContainsAny(sessionID, "/\\")
}
