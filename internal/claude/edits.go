package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/editpreview"
)

// LatestPreview reads public activity and completed edits in one incremental pass.
func (adapter Adapter) LatestPreview(ctx context.Context, session agent.Session) (agent.Preview, error) {
	id := session.TranscriptID
	if id == "" {
		id = session.ID
	}
	if strings.ContainsAny(id, "/\\:*?[]") || adapter.edits == nil {
		return agent.Preview{}, fmt.Errorf("Claude transcript identity unavailable")
	}
	root := os.Getenv("CLAUDE_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return agent.Preview{}, err
		}
		root = filepath.Join(home, ".claude")
	}
	// Match names only; never read other sessions to discover this transcript.
	paths, err := filepath.Glob(filepath.Join(root, "projects", "*", id+".jsonl"))
	if err != nil {
		return agent.Preview{}, err
	}
	if len(paths) != 1 {
		return agent.Preview{}, fmt.Errorf("Claude transcript unavailable or ambiguous")
	}
	return adapter.edits.Read(ctx, session.ID, paths[0], func() editpreview.Decoder { return &editDecoder{sessionID: id, pending: make(map[string]string)} })
}

type editDecoder struct {
	sessionID string
	pending   map[string]string
	activity  *agent.Activity
}

func (decoder *editDecoder) Decode(line []byte) (*agent.Edit, error) {
	var record struct {
		Type      string    `json:"type"`
		SessionID string    `json:"sessionId"`
		Sidechain bool      `json:"isSidechain"`
		Timestamp time.Time `json:"timestamp"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Result json.RawMessage `json:"toolUseResult"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, err
	}
	if record.SessionID != "" && record.SessionID != decoder.sessionID {
		return nil, fmt.Errorf("Claude transcript identity mismatch")
	}
	if record.Sidechain {
		return nil, nil
	}
	var blocks []struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		ToolUseID string          `json:"tool_use_id"`
		IsError   bool            `json:"is_error"`
		Text      string          `json:"text"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(record.Message.Content, &blocks) != nil {
		return nil, nil
	}
	for _, block := range blocks {
		if record.Type == "assistant" {
			switch block.Type {
			case "text":
				decoder.activity = editpreview.PublicActivity("Assistant", block.Text, record.Timestamp)
			case "tool_use":
				decoder.activity = editpreview.PublicActivity("Tool requested", block.Name, record.Timestamp)
			}
		}
		if record.Type == "user" && block.Type == "tool_result" {
			var text string
			if json.Unmarshal(block.Content, &text) != nil {
				var content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if json.Unmarshal(block.Content, &content) == nil {
					for _, part := range content {
						if part.Type == "text" {
							text += part.Text + "\n"
						}
					}
				}
			}
			if text != "" {
				label := "Tool result"
				if block.IsError {
					label = "Tool failed"
				}
				decoder.activity = editpreview.PublicActivity(label, text, record.Timestamp)
			}
		}
		if record.Type == "assistant" && block.Type == "tool_use" && (block.Name == "Edit" || block.Name == "Write") {
			decoder.pending[block.ID] = block.Name
		}
		if record.Type != "user" || block.Type != "tool_result" {
			continue
		}
		name, ok := decoder.pending[block.ToolUseID]
		delete(decoder.pending, block.ToolUseID)
		if !ok || block.IsError {
			continue
		}
		var result struct {
			FilePath     string  `json:"filePath"`
			Type         string  `json:"type"`
			Content      string  `json:"content"`
			OriginalFile *string `json:"originalFile"`
			OldString    string  `json:"oldString"`
			NewString    string  `json:"newString"`
			Staged       bool    `json:"staged"`
			Patch        []struct {
				OldStart int      `json:"oldStart"`
				OldLines int      `json:"oldLines"`
				NewStart int      `json:"newStart"`
				NewLines int      `json:"newLines"`
				Lines    []string `json:"lines"`
			} `json:"structuredPatch"`
		}
		if json.Unmarshal(record.Result, &result) != nil || result.Staged || result.FilePath == "" {
			continue
		}
		var hunks []string
		for _, h := range result.Patch {
			hunks = append(hunks, fmt.Sprintf("@@ -%d,%d +%d,%d @@\n%s", h.OldStart, h.OldLines, h.NewStart, h.NewLines, strings.Join(h.Lines, "\n")))
		}
		if len(hunks) == 0 && name == "Write" && result.Type == "create" {
			hunks = append(hunks, "+"+strings.ReplaceAll(strings.TrimSuffix(result.Content, "\n"), "\n", "\n+"))
		}
		if len(hunks) == 0 {
			// An empty patch can mean a no-op or a diff that was too large/timed out.
			// Do not silently retain an older edit when a newer write lacks its diff.
			unchanged := name == "Edit" && result.OldString == result.NewString
			if name == "Write" {
				unchanged = result.OriginalFile != nil && *result.OriginalFile == result.Content
			}
			if unchanged {
				continue
			}
		}
		return &agent.Edit{Filename: result.FilePath, CompletedAt: record.Timestamp, Diff: strings.Join(hunks, "\n")}, nil
	}
	return nil, nil
}

func (decoder *editDecoder) Activity() *agent.Activity { return decoder.activity }
