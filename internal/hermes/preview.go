package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/editpreview"
)

type publicPreview struct {
	SessionID string `json:"sessionID"`
	Directory string `json:"directory"`
	Activity  *struct {
		Label string `json:"label"`
		Text  string `json:"text"`
		At    int64  `json:"at"`
	} `json:"activity"`
	Edit *struct {
		Filename  string `json:"filename"`
		Diff      string `json:"diff"`
		At        int64  `json:"at"`
		Truncated bool   `json:"truncated"`
	} `json:"edit"`
}

// LatestPreview reads only the selected process's live public snapshot. No saved
// transcript, earlier branch, or workspace diff contributes to this preview.
func (adapter Adapter) LatestPreview(ctx context.Context, session agent.Session) (agent.Preview, error) {
	if session.Provider != "hermes" || session.ProcessID == nil || session.TranscriptID == "" {
		return agent.Preview{}, fmt.Errorf("Hermes live preview unavailable; restart Hermes to load the a-gent plugin")
	}
	instances, err := adapter.discover(ctx)
	if err != nil {
		return agent.Preview{}, err
	}
	for _, instance := range instances {
		if instance.pid != *session.ProcessID {
			continue
		}
		record, err := adapter.runtime(instance)
		if err != nil {
			return agent.Preview{}, err
		}
		if session.ID != instance.identity()+":"+record.SessionID || session.TranscriptID != record.SessionID ||
			session.WorkingDirectory != record.Directory {
			return agent.Preview{}, fmt.Errorf("Hermes session changed")
		}
		file, err := os.Open(filepath.Join(adapter.directory, "previews", strconv.Itoa(instance.pid)+".json"))
		if err != nil {
			return agent.Preview{}, err
		}
		defer file.Close()
		var value publicPreview
		if err := json.NewDecoder(io.LimitReader(file, 512*1024)).Decode(&value); err != nil {
			return agent.Preview{}, fmt.Errorf("decode Hermes public preview: %w", err)
		}
		if value.SessionID != record.SessionID || value.Directory != record.Directory {
			return agent.Preview{}, fmt.Errorf("Hermes preview does not match the selected session")
		}
		var preview agent.Preview
		if value.Activity != nil {
			preview.Activity = editpreview.PublicActivity(
				value.Activity.Label, value.Activity.Text, time.UnixMilli(value.Activity.At),
			)
		}
		if value.Edit != nil && value.Edit.Filename != "" && value.Edit.Diff != "" {
			diff := value.Edit.Diff
			filename := value.Edit.Filename
			// Hermes returns absolute paths in patch results. Shorten only the
			// native headers for files inside this session's working directory.
			if filepath.IsAbs(filename) {
				relative, err := filepath.Rel(record.Directory, filename)
				if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					diff = strings.Replace(diff, "--- a/"+filename+"\n", "--- a/"+relative+"\n", 1)
					diff = strings.Replace(diff, "+++ b/"+filename+"\n", "+++ b/"+relative+"\n", 1)
					filename = relative
				}
			}
			truncated := value.Edit.Truncated || len(diff) > 64*1024
			if len(diff) > 64*1024 {
				diff = diff[:64*1024]
			}
			preview.Edit = &agent.Edit{Filename: filename, Diff: diff,
				CompletedAt: time.UnixMilli(value.Edit.At), Truncated: truncated}
		}
		return preview, ctx.Err()
	}
	return agent.Preview{}, fmt.Errorf("Hermes process is no longer live")
}
