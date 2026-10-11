// Package pi observes live Pi processes and their native extension lifecycle.
package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"a-gent/internal/agent"
)

// Adapter reads metadata published by the bundled Pi observer extension.
type Adapter struct {
	discover  func(context.Context) ([]instance, error)
	directory string
	now       func() time.Time
	preview   *previewCache
	setupErr  error
}

// NewAdapter discovers Pi without reading saved sessions as live instances.
func NewAdapter() Adapter {
	directory, _ := observerDirectory()
	return Adapter{discover: discoverInstances, directory: directory, now: time.Now, preview: &previewCache{}}
}

// EnableObserver configures normal Pi launches to publish native live status.
// Sessions reports setup failures through the normal provider error path.
func (adapter *Adapter) EnableObserver() {
	if _, err := exec.LookPath("pi"); err != nil {
		return
	}
	_, adapter.setupErr = ConfigureObserver()
}

// Provider returns the provider name used by launching, navigation, and previews.
func (Adapter) Provider() string { return "pi" }

type runtimeRecord struct {
	Version      int         `json:"version"`
	PID          int         `json:"pid"`
	StartedAt    int64       `json:"startedAt"`
	UpdatedAt    int64       `json:"updatedAt"`
	LastActiveAt int64       `json:"lastActiveAt"`
	Directory    string      `json:"directory"`
	SessionID    string      `json:"sessionID"`
	SessionFile  string      `json:"sessionFile"`
	LeafID       string      `json:"leafID"`
	Name         string      `json:"name"`
	State        agent.State `json:"state"`
}

func (adapter Adapter) runtime(instance instance) (runtimeRecord, error) {
	var record runtimeRecord
	if adapter.directory == "" {
		return record, fmt.Errorf("Pi observer directory unavailable")
	}
	path := filepath.Join(adapter.directory, "instances", strconv.Itoa(instance.pid)+".json")
	file, err := os.Open(path)
	if err != nil {
		return record, fmt.Errorf("Pi observer is not loaded; run /reload in Pi or restart Pi")
	}
	defer file.Close()
	if err := json.NewDecoder(io.LimitReader(file, 64*1024)).Decode(&record); err != nil {
		return record, fmt.Errorf("decode Pi observer: %w", err)
	}
	if !record.matches(instance) {
		return record, fmt.Errorf("Pi observer metadata does not match the live process")
	}
	age := adapter.now().Sub(time.UnixMilli(record.UpdatedAt))
	if age > 5*time.Second || age < -2*time.Second {
		return record, fmt.Errorf("Pi observer metadata is stale")
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

// Sessions reports only live processes. Persisted observer files and Pi's saved
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
		entry := agent.Session{ID: instance.identity(), Provider: "pi", Name: "Pi",
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
			entry.TranscriptPath = record.SessionFile
			entry.LastActiveAt = time.UnixMilli(record.LastActiveAt)
			// Pi has no universal event for extension-owned approval dialogs.
			// Its native idle predicate tells us whether an agent run is ongoing.
			if err == nil && (record.State == agent.StateRunning || record.State == agent.StateIdle) {
				entry.State = record.State
			}
		}
		sessions = append(sessions, entry)
	}
	return sessions, nil
}
