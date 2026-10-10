package opencode

import (
	"context"
	"fmt"
	"strings"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/editpreview"
)

type messageRecord struct {
	Info struct {
		SessionID string `json:"sessionID"`
		Role      string `json:"role"`
		Time      struct {
			Created int64 `json:"created"`
		} `json:"time"`
	} `json:"info"`
	Parts []struct {
		SessionID string `json:"sessionID"`
		Type      string `json:"type"`
		Text      string `json:"text"`
		Synthetic bool   `json:"synthetic"`
		Ignored   bool   `json:"ignored"`
		Tool      string `json:"tool"`
		Time      struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"time"`
		State struct {
			Status string `json:"status"`
			Title  string `json:"title"`
			Output string `json:"output"`
			Time   struct {
				End int64 `json:"end"`
			} `json:"time"`
			Metadata struct {
				Diff     string `json:"diff"`
				Filepath string `json:"filepath"`
				Filediff struct {
					File  string `json:"file"`
					Patch string `json:"patch"`
				} `json:"filediff"`
			} `json:"metadata"`
		} `json:"state"`
	} `json:"parts"`
}

// LatestPreview reads public messages for the selected session only. Completed
// edit-tool metadata supplies attributable diffs; workspace git changes do not.
func (adapter Adapter) LatestPreview(ctx context.Context, session agent.Session) (agent.Preview, error) {
	if session.Provider != "opencode" || session.TranscriptID == "" || strings.ContainsAny(session.TranscriptID, "/\\?#%") {
		return agent.Preview{}, fmt.Errorf("OpenCode session preview unavailable")
	}
	var messages []messageRecord
	if err := adapter.get(ctx, session.TranscriptPath, session.WorkingDirectory,
		"/session/"+session.TranscriptID+"/message", &messages); err != nil {
		return agent.Preview{}, err
	}
	var preview agent.Preview
	for _, message := range messages {
		if message.Info.Role != "assistant" || message.Info.SessionID != session.TranscriptID {
			continue
		}
		for _, part := range message.Parts {
			if part.SessionID != session.TranscriptID {
				continue
			}
			switch part.Type {
			case "text":
				if part.Synthetic || part.Ignored || strings.TrimSpace(part.Text) == "" {
					continue
				}
				at := part.Time.End
				if at == 0 {
					at = part.Time.Start
				}
				if at == 0 {
					at = message.Info.Time.Created
				}
				activity := editpreview.PublicActivity("Assistant", part.Text, time.UnixMilli(at))
				if preview.Activity == nil || !activity.At.Before(preview.Activity.At) {
					preview.Activity = activity
				}
			case "tool":
				if part.State.Status != "completed" || part.State.Time.End <= 0 {
					continue
				}
				at := time.UnixMilli(part.State.Time.End)
				text := part.State.Output
				if text == "" {
					text = part.State.Title
				}
				if text != "" && (preview.Activity == nil || !at.Before(preview.Activity.At)) {
					preview.Activity = editpreview.PublicActivity(part.Tool, text, at)
				}
				metadata := part.State.Metadata
				if metadata.Filepath == "" {
					metadata.Filepath = metadata.Filediff.File
				}
				if metadata.Diff == "" {
					metadata.Diff = metadata.Filediff.Patch
				}
				if (part.Tool == "edit" || part.Tool == "write") && metadata.Filepath != "" && metadata.Diff != "" &&
					(preview.Edit == nil || !at.Before(preview.Edit.CompletedAt)) {
					const diffLimit = 64 * 1024
					truncated := len(metadata.Diff) > diffLimit
					if truncated {
						metadata.Diff = strings.ToValidUTF8(metadata.Diff[:diffLimit], "")
					}
					preview.Edit = &agent.Edit{
						Filename: metadata.Filepath, Diff: metadata.Diff, CompletedAt: at, Truncated: truncated,
					}
				}
			}
		}
	}
	return preview, ctx.Err()
}
