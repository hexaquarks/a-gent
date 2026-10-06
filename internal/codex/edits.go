package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/editpreview"
)

// LatestPreview reads public activity and completed edits in one incremental pass.
func (adapter Adapter) LatestPreview(ctx context.Context, session agent.Session) (agent.Preview, error) {
	if session.TranscriptPath == "" || adapter.edits == nil {
		return agent.Preview{}, fmt.Errorf("Codex transcript unavailable")
	}
	return adapter.edits.Read(ctx, session.ID, session.TranscriptPath, func() editpreview.Decoder {
		return &editDecoder{sessionID: session.ID, pending: make(map[string]string)}
	})
}

type editDecoder struct {
	sessionID string
	pending   map[string]string
	activity  *agent.Activity
}

func (decoder *editDecoder) Decode(line []byte) (*agent.Edit, error) {
	var record struct {
		Type      string    `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		Payload   struct {
			Type        string          `json:"type"`
			ID          string          `json:"id"`
			ThreadID    string          `json:"thread_id"`
			CompletedAt int64           `json:"completed_at_ms"`
			Name        string          `json:"name"`
			CallID      string          `json:"call_id"`
			Input       string          `json:"input"`
			Output      json.RawMessage `json:"output"`
			Role        string          `json:"role"`
			Phase       string          `json:"phase"`
			Content     []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Item struct {
				Type    string `json:"type"`
				Status  string `json:"status"`
				Phase   string `json:"phase"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				Command []string `json:"command"`
				Output  string   `json:"aggregated_output"`
				Changes map[string]struct {
					Type    string `json:"type"`
					Diff    string `json:"unified_diff"`
					Content string `json:"content"`
				} `json:"changes"`
			} `json:"item"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, err
	}
	payload := record.Payload
	if record.Type == "session_meta" && payload.ID != decoder.sessionID {
		return nil, fmt.Errorf("Codex transcript identity mismatch")
	}
	// Only public assistant messages and tool events belong in the feed.
	// Reasoning/encrypted blocks and inter-agent messages are intentionally ignored.
	if record.Type == "event_msg" && payload.Type == "item_completed" && payload.ThreadID == decoder.sessionID {
		switch payload.Item.Type {
		case "AgentMessage":
			if payload.Item.Phase != "analysis" {
				var text []string
				for _, block := range payload.Item.Content {
					if block.Type == "Text" {
						text = append(text, block.Text)
					}
				}
				if len(text) > 0 {
					decoder.activity = editpreview.PublicActivity("Assistant", strings.Join(text, "\n"), record.Timestamp)
				}
			}
		case "CommandExecution":
			text := strings.Join(payload.Item.Command, " ")
			if payload.Item.Output != "" {
				text += "\n" + payload.Item.Output
			}
			decoder.activity = editpreview.PublicActivity("Command · "+payload.Item.Status, text, record.Timestamp)
		}
	}
	if record.Type == "response_item" && payload.Type == "message" && payload.Role == "assistant" && payload.Phase != "analysis" {
		var text []string
		for _, block := range payload.Content {
			if block.Type == "output_text" {
				text = append(text, block.Text)
			}
		}
		if len(text) > 0 {
			decoder.activity = editpreview.PublicActivity("Assistant", strings.Join(text, "\n"), record.Timestamp)
		}
	}
	if record.Type == "response_item" && (payload.Type == "function_call" || payload.Type == "custom_tool_call") {
		decoder.activity = editpreview.PublicActivity("Tool requested", payload.Name, record.Timestamp)
	}
	if record.Type == "event_msg" && payload.Type == "item_completed" && payload.ThreadID == decoder.sessionID && payload.Item.Type == "FileChange" && payload.Item.Status == "completed" {
		paths := make([]string, 0, len(payload.Item.Changes))
		for path := range payload.Item.Changes {
			paths = append(paths, path)
		}
		sort.Strings(paths) // A multi-file completion has no per-file ordering.
		for _, path := range paths {
			change := payload.Item.Changes[path]
			diff := change.Diff
			if change.Type == "add" || change.Type == "delete" {
				prefix := "+"
				if change.Type == "delete" {
					prefix = "-"
				}
				diff = prefix + strings.ReplaceAll(strings.TrimSuffix(change.Content, "\n"), "\n", "\n"+prefix)
			}
			completed := record.Timestamp
			if payload.CompletedAt > 0 {
				completed = time.UnixMilli(payload.CompletedAt)
			}
			return &agent.Edit{Filename: path, CompletedAt: completed, Diff: diff, Truncated: len(paths) > 1}, nil
		}
	}
	// Older rollouts contain raw apply_patch requests and their matching outputs.
	// A request's own status=completed does not establish that the patch succeeded.
	if record.Type != "response_item" {
		return nil, nil
	}
	if payload.Type == "custom_tool_call" && payload.Name == "apply_patch" {
		decoder.pending[payload.CallID] = payload.Input
	}
	if payload.Type != "custom_tool_call_output" {
		return nil, nil
	}
	patch, ok := decoder.pending[payload.CallID]
	delete(decoder.pending, payload.CallID)
	if !ok {
		return nil, nil
	}
	var result struct {
		Output   string `json:"output"`
		Metadata struct {
			ExitCode *int `json:"exit_code"`
		} `json:"metadata"`
	}
	var output string
	if json.Unmarshal(payload.Output, &output) != nil {
		return nil, nil
	}
	if json.Unmarshal([]byte(output), &result) != nil || result.Metadata.ExitCode == nil || *result.Metadata.ExitCode != 0 || !strings.HasPrefix(result.Output, "Success. Updated the following files:") {
		return nil, nil
	}
	var filename string
	var lines []string
	truncated := false
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "*** Update File: ") || strings.HasPrefix(line, "*** Add File: ") || strings.HasPrefix(line, "*** Delete File: ") {
			if filename != "" {
				truncated = true
				break
			}
			filename = strings.SplitN(line, ": ", 2)[1]
		} else if filename != "" && !strings.HasPrefix(line, "***") {
			lines = append(lines, line)
		}
	}
	if filename == "" {
		return nil, nil
	}
	return &agent.Edit{Filename: filename, CompletedAt: record.Timestamp, Diff: strings.Join(lines, "\n"), Truncated: truncated}, nil
}

func (decoder *editDecoder) Activity() *agent.Activity { return decoder.activity }
