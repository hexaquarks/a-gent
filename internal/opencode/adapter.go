// Package opencode reads live OpenCode instances through their local HTTP API.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"a-gent/internal/agent"
)

// Adapter implements read-only discovery and status for local OpenCode processes.
type Adapter struct {
	discover func(context.Context) ([]instance, error)
	client   *http.Client
	state    *adapterState
}

type adapterState struct {
	sync.Mutex
	sessions map[string][]agent.Session
}

// NewAdapter discovers existing instances, including randomly assigned ports.
func NewAdapter() Adapter {
	client := &http.Client{
		Timeout: time.Second,
		// Local provider traffic must not pass through an environment HTTP proxy.
		Transport:     &http.Transport{DialContext: (&net.Dialer{Timeout: time.Second}).DialContext},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	return Adapter{
		discover: discoverInstances,
		client:   client,
		state:    &adapterState{sessions: make(map[string][]agent.Session)},
	}
}

// Provider returns the name shared by discovery, previews, and launching.
func (Adapter) Provider() string { return "opencode" }

type sessionRecord struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentID"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	Time      struct {
		Created  int64 `json:"created"`
		Updated  int64 `json:"updated"`
		Archived int64 `json:"archived"`
	} `json:"time"`
}

type statusRecord struct {
	Type string `json:"type"`
}

type pendingRequest struct {
	SessionID string `json:"sessionID"`
}

// Sessions returns live instances and sessions observed in their own runtime.
func (adapter Adapter) Sessions(ctx context.Context) ([]agent.Session, error) {
	instances, err := adapter.discover(ctx)
	if err != nil {
		return nil, err
	}

	adapter.state.Lock()
	defer adapter.state.Unlock()
	// Independent local servers can stall. Read them together so several slow
	// instances cannot consume the whole poll deadline before healthy ones run.
	results := make([][]agent.Session, len(instances))
	var workers sync.WaitGroup
	for index, instance := range instances {
		previous := adapter.state.sessions[instance.identity()]
		workers.Add(1)
		go func() {
			defer workers.Done()
			entries, err := adapter.instanceSessions(ctx, instance, previous)
			if err != nil {
				entries = cloneSessions(previous)
				if len(entries) == 0 {
					entries = []agent.Session{instance.placeholder(agent.StateUnavailable, err.Error())}
				}
				for i := range entries {
					entries[i].State = agent.StateUnavailable
					entries[i].Preview = err.Error()
				}
			}
			results[index] = entries
		}()
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	next := make(map[string][]agent.Session)
	var sessions []agent.Session
	for index, instance := range instances {
		entries := results[index]
		next[instance.identity()] = cloneSessions(entries)
		sessions = append(sessions, entries...)
	}
	adapter.state.sessions = next
	return sessions, nil
}

func (instance instance) placeholder(state agent.State, preview string) agent.Session {
	return agent.Session{
		ID:               instance.identity(),
		Provider:         "opencode",
		Name:             "OpenCode",
		Preview:          preview,
		WorkingDirectory: instance.directory,
		ProcessID:        &instance.pid,
		State:            state,
	}
}

func (adapter Adapter) instanceSessions(
	ctx context.Context, instance instance, previous []agent.Session,
) ([]agent.Session, error) {
	if instance.endpoint == "" {
		return nil, fmt.Errorf("Status requires OpenCode's local API; start with opencode --port 0")
	}
	if instance.directory == "" {
		return nil, fmt.Errorf("Cannot identify OpenCode's working directory")
	}
	var records []sessionRecord
	if err := adapter.get(ctx, instance.endpoint, instance.directory, "/session", &records); err != nil {
		return nil, err
	}
	var statuses map[string]statusRecord
	if err := adapter.get(ctx, instance.endpoint, instance.directory, "/session/status", &statuses); err != nil {
		return nil, err
	}
	waiting := make(map[string]bool)
	for _, path := range []string{"/permission", "/question"} {
		var pending []pendingRequest
		if err := adapter.get(ctx, instance.endpoint, instance.directory, path, &pending); err != nil {
			return nil, err
		}
		for _, request := range pending {
			waiting[request.SessionID] = true
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Time.Updated > records[j].Time.Updated })
	var sessions []agent.Session
	known := make(map[string]bool)
	for _, session := range previous {
		if session.TranscriptID != "" {
			known[session.TranscriptID] = true
		}
	}
	continued := false
	for _, record := range records {
		if record.ID == "" || record.ParentID != "" || record.Time.Archived > 0 || record.Directory != instance.directory {
			continue
		}
		status, active := statuses[record.ID]
		explicit := record.ID == instance.sessionID
		if instance.continuing && !continued && len(known) == 0 {
			explicit = true
			continued = true
		}
		// Session history is shared across processes, even in the same project.
		// Runtime status or an explicit resume establishes ownership; retain those
		// sessions after OpenCode removes their idle entries from the status map.
		if !active && !waiting[record.ID] && !explicit && !known[record.ID] {
			continue
		}
		state := agent.StateIdle // OpenCode removes idle sessions from its status map.
		if active {
			switch status.Type {
			case "busy", "retry":
				state = agent.StateRunning
			case "idle":
				state = agent.StateIdle
			default:
				state = agent.StateUnavailable
			}
		}
		if waiting[record.ID] {
			state = agent.StateWaiting
		}
		sessions = append(sessions, agent.Session{
			ID: instance.identity() + ":" + record.ID, TranscriptID: record.ID, TranscriptPath: instance.endpoint,
			Provider: "opencode", Name: record.Title, WorkingDirectory: record.Directory,
			ProcessID: &instance.pid, State: state, LastActiveAt: time.UnixMilli(record.Time.Updated),
		})
	}
	if len(sessions) == 0 {
		return []agent.Session{instance.placeholder(agent.StateIdle, "Ready for a new session")}, nil
	}
	return sessions, nil
}

func (adapter Adapter) get(ctx context.Context, endpoint, directory, path string, target any) error {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "http" || base.User != nil || base.RawQuery != "" || base.Path != "" {
		return fmt.Errorf("Invalid OpenCode local endpoint")
	}
	ip := net.ParseIP(base.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("OpenCode endpoint must be on loopback")
	}
	base.Path = path
	query := url.Values{"directory": []string{directory}}
	base.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return err
	}
	if password := os.Getenv("OPENCODE_SERVER_PASSWORD"); password != "" {
		username := os.Getenv("OPENCODE_SERVER_USERNAME")
		if username == "" {
			username = "opencode"
		}
		request.SetBasicAuth(username, password)
	}
	response, err := adapter.client.Do(request)
	if err != nil {
		return fmt.Errorf("Read OpenCode local API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenCode %s returned HTTP %d", path, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return fmt.Errorf("OpenCode response exceeds 8 MiB")
	}
	if strings.TrimSpace(string(data)) == "null" {
		return fmt.Errorf("OpenCode %s returned null", path)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("Decode OpenCode %s: %w", path, err)
	}
	return nil
}

// ProcessID is a pointer in the shared session type; caches must own their copy.
func cloneSessions(sessions []agent.Session) []agent.Session {
	copies := append([]agent.Session(nil), sessions...)
	for i := range copies {
		if copies[i].ProcessID != nil {
			pid := *copies[i].ProcessID
			copies[i].ProcessID = &pid
		}
	}
	return copies
}
