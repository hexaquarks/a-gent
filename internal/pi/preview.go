package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/editpreview"
)

// Cache the selected preview, rather than repeatedly parsing an unchanged tree
// on every UI refresh. A changed leaf invalidates it even without a file append.
type previewCache struct {
	sync.Mutex
	path      string
	sessionID string
	directory string
	leaf      string
	info      os.FileInfo
	value     agent.Preview
}

func (cache *previewCache) read(ctx context.Context, record runtimeRecord) (agent.Preview, error) {
	cache.Lock()
	defer cache.Unlock()
	if err := ctx.Err(); err != nil {
		return agent.Preview{}, err
	}
	info, err := os.Stat(record.SessionFile)
	if err != nil {
		return agent.Preview{}, err
	}
	if cache.info != nil && cache.path == record.SessionFile && cache.sessionID == record.SessionID &&
		cache.directory == record.Directory && cache.leaf == record.LeafID &&
		os.SameFile(cache.info, info) && cache.info.Size() == info.Size() && cache.info.ModTime().Equal(info.ModTime()) {
		return cache.value, nil
	}
	value, err := readPreview(ctx, record)
	if err != nil {
		return agent.Preview{}, err
	}
	cache.path, cache.leaf, cache.info, cache.value = record.SessionFile, record.LeafID, info, value
	cache.sessionID, cache.directory = record.SessionID, record.Directory
	return value, nil
}

type transcriptEntry struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	ParentID  string    `json:"parentId"`
	Timestamp time.Time `json:"timestamp"`
	Directory string    `json:"cwd"`
	Message   struct {
		Role       string          `json:"role"`
		ToolCallID string          `json:"toolCallId"`
		ToolName   string          `json:"toolName"`
		IsError    bool            `json:"isError"`
		Content    json.RawMessage `json:"content"`
		Details    json.RawMessage `json:"details"`
	} `json:"message"`
}

// LatestPreview follows Pi's active leaf through the native transcript tree.
// Saved and abandoned branches cannot contribute activity or completed edits.
func (adapter Adapter) LatestPreview(ctx context.Context, session agent.Session) (agent.Preview, error) {
	if session.Provider != "pi" || session.ProcessID == nil || session.TranscriptID == "" {
		return agent.Preview{}, fmt.Errorf("Pi session preview unavailable; run /reload in Pi or restart Pi")
	}
	instances, err := adapter.discover(ctx)
	if err != nil {
		return agent.Preview{}, err
	}
	var record runtimeRecord
	found := false
	for _, instance := range instances {
		if instance.pid != *session.ProcessID {
			continue
		}
		record, err = adapter.runtime(instance)
		if err != nil {
			return agent.Preview{}, err
		}
		found = session.ID == instance.identity()+":"+record.SessionID &&
			session.TranscriptID == record.SessionID && session.TranscriptPath == record.SessionFile &&
			session.WorkingDirectory == record.Directory
		break
	}
	if !found || !filepath.IsAbs(record.SessionFile) {
		return agent.Preview{}, fmt.Errorf("Pi session changed or its transcript is unavailable")
	}
	if record.LeafID == "" {
		return agent.Preview{}, nil
	}
	if adapter.preview != nil {
		return adapter.preview.read(ctx, record)
	}
	return readPreview(ctx, record)
}

func readPreview(ctx context.Context, record runtimeRecord) (agent.Preview, error) {
	file, err := os.Open(record.SessionFile)
	if err != nil {
		return agent.Preview{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return agent.Preview{}, err
	}
	// Pi persists a tree, so branch selection requires an index rather than a
	// linear tail. Bound the read to the file size observed and cap large histories.
	if info.Size() > 64<<20 {
		return agent.Preview{}, fmt.Errorf("Pi transcript exceeds 64 MiB")
	}
	lines := bufio.NewReader(io.LimitReader(file, info.Size()))
	entries := make(map[string]transcriptEntry)
	header := false
	for {
		if err := ctx.Err(); err != nil {
			return agent.Preview{}, err
		}
		line, err := lines.ReadBytes('\n')
		if err == io.EOF {
			break // Retry an incomplete append on the next refresh.
		}
		if err != nil {
			return agent.Preview{}, err
		}
		var entry transcriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return agent.Preview{}, fmt.Errorf("decode Pi transcript: %w", err)
		}
		if !header {
			if entry.Type != "session" || entry.ID != record.SessionID || entry.Directory != record.Directory {
				return agent.Preview{}, fmt.Errorf("Pi transcript does not match the selected session")
			}
			header = true
			continue
		}
		if entry.ID != "" {
			entries[entry.ID] = entry
		}
	}
	var branch []transcriptEntry
	seen := make(map[string]bool)
	for id := record.LeafID; id != ""; {
		entry, exists := entries[id]
		if !exists || seen[id] {
			return agent.Preview{}, fmt.Errorf("Pi active branch is incomplete or invalid")
		}
		seen[id] = true
		branch = append(branch, entry)
		id = entry.ParentID
	}
	var preview agent.Preview
	type toolCall struct{ name, path string }
	calls := make(map[string]toolCall)
	for i := len(branch) - 1; i >= 0; i-- {
		entry := branch[i]
		if entry.Type != "message" {
			continue
		}
		message := entry.Message
		if message.Role != "assistant" && message.Role != "toolResult" {
			continue
		}
		var parts []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(message.Content, &parts); err != nil {
			return agent.Preview{}, fmt.Errorf("decode Pi public message: %w", err)
		}
		var text []string
		for _, part := range parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				text = append(text, part.Text)
			}
			if message.Role == "assistant" && part.Type == "toolCall" && part.ID != "" {
				var arguments struct {
					Path string `json:"path"`
				}
				if part.Name == "edit" {
					json.Unmarshal(part.Arguments, &arguments)
				}
				calls[part.ID] = toolCall{name: part.Name, path: arguments.Path}
			}
		}
		if message.Role == "assistant" && len(text) > 0 {
			preview.Activity = editpreview.PublicActivity("Assistant", strings.Join(text, "\n"), entry.Timestamp)
		}
		if message.Role != "toolResult" {
			continue
		}
		call, matched := calls[message.ToolCallID]
		delete(calls, message.ToolCallID)
		if !matched || call.name != message.ToolName {
			continue
		}
		if len(text) > 0 {
			preview.Activity = editpreview.PublicActivity(message.ToolName, strings.Join(text, "\n"), entry.Timestamp)
		}
		// Native edit results include the applied diff. Writes without a native
		// diff still appear as activity; their requests do not prove a file change.
		if message.IsError || call.name != "edit" || call.path == "" {
			continue
		}
		var details struct {
			Diff  string `json:"diff"`
			Patch string `json:"patch"`
		}
		if json.Unmarshal(message.Details, &details) != nil {
			continue
		}
		diff := details.Patch
		if diff == "" {
			diff = details.Diff
		}
		if diff == "" {
			continue
		}
		truncated := len(diff) > 64*1024
		if truncated {
			diff = strings.ToValidUTF8(diff[:64*1024], "")
		}
		preview.Edit = &agent.Edit{Filename: call.path, Diff: diff, CompletedAt: entry.Timestamp, Truncated: truncated}
	}
	return preview, ctx.Err()
}
