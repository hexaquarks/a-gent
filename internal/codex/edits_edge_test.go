package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestCodexPreviewRequiresTranscriptReaderAndPath(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		adapter Adapter
		session agent.Session
	}{
		{name: "missing path", adapter: NewAdapter(), session: agent.Session{ID: "one"}},
		{name: "missing reader", adapter: Adapter{}, session: agent.Session{ID: "one", TranscriptPath: "rollout.jsonl"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			preview, err := testCase.adapter.LatestPreview(context.Background(), testCase.session)
			if err == nil || err.Error() != "Codex transcript unavailable" || preview.Edit != nil || preview.Activity != nil {
				t.Fatalf("preview = %+v, error = %v", preview, err)
			}
		})
	}
}

func TestCodexDecoderRejectsMalformedAndForeignMetadata(t *testing.T) {
	for _, record := range []string{`{`, `{"type":"session_meta","payload":{"id":"other"}}`} {
		decoder := &editDecoder{sessionID: "one", pending: make(map[string]string)}
		edit, err := decoder.Decode([]byte(record))
		if err == nil || edit != nil || decoder.Activity() != nil {
			t.Fatalf("record %s: edit = %+v, activity = %+v, error = %v", record, edit, decoder.Activity(), err)
		}
	}
}

func TestCodexPublicAgentMessageKeepsOnlyTextBlocks(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: make(map[string]string)}
	record := `{"type":"event_msg","timestamp":"2026-10-05T12:00:00Z","payload":{"type":"item_completed","thread_id":"one","item":{"type":"AgentMessage","phase":"commentary","content":[{"type":"Text","text":"First"},{"type":"Reasoning","text":"Private"},{"type":"Text","text":"Second"}]}}}`
	if _, err := decoder.Decode([]byte(record)); err != nil {
		t.Fatal(err)
	}
	activity := decoder.Activity()
	if activity == nil || activity.Text != "First\nSecond" || activity.Label != "Assistant" {
		t.Fatalf("activity = %+v", activity)
	}

	for _, ignored := range []string{
		strings.Replace(record, `"phase":"commentary"`, `"phase":"analysis"`, 1),
		`{"type":"event_msg","payload":{"type":"item_completed","thread_id":"one","item":{"type":"AgentMessage","content":[{"type":"Reasoning","text":"Private"}]}}}`,
	} {
		if _, err := decoder.Decode([]byte(ignored)); err != nil {
			t.Fatal(err)
		}
		if decoder.Activity() != activity {
			t.Fatalf("ignored message replaced activity: %+v", decoder.Activity())
		}
	}
}

func TestCodexFileChangeSelectsFirstPathAndUsesRecordTimestamp(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: make(map[string]string)}
	record := `{"type":"event_msg","timestamp":"2026-10-05T12:00:00Z","payload":{"type":"item_completed","thread_id":"one","item":{"type":"FileChange","status":"completed","changes":{"z.go":{"type":"add","content":"later\n"},"a.go":{"type":"delete","content":"first\nsecond\n"}}}}}`
	edit, err := decoder.Decode([]byte(record))
	if err != nil || edit == nil {
		t.Fatalf("edit = %+v, error = %v", edit, err)
	}
	expectedTime := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	if edit.Filename != "a.go" || edit.Diff != "-first\n-second" || !edit.Truncated || !edit.CompletedAt.Equal(expectedTime) {
		t.Fatalf("edit = %+v", edit)
	}
}

func TestCodexPatchResultsConsumePendingRequests(t *testing.T) {
	const successfulOutput = `{"output":"Success. Updated the following files:\nM a.go\n","metadata":{"exit_code":0}}`
	testCases := []struct {
		name     string
		patch    string
		output   any
		wantDiff string
	}{
		{name: "non-string result", patch: "*** Add File: a.go\n+new", output: map[string]any{"output": "unexpected"}},
		{name: "malformed nested result", patch: "*** Add File: a.go\n+new", output: "{"},
		{name: "missing filename", patch: "*** Begin Patch\n*** End Patch", output: successfulOutput},
		{
			name: "multiple files", output: successfulOutput, wantDiff: "+first",
			patch: "*** Begin Patch\n*** Add File: a.go\n+first\n*** Add File: b.go\n+second\n*** End Patch",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			decoder := &editDecoder{sessionID: "one", pending: map[string]string{"patch": testCase.patch}}
			record, err := json.Marshal(map[string]any{
				"type":    "response_item",
				"payload": map[string]any{"type": "custom_tool_call_output", "call_id": "patch", "output": testCase.output},
			})
			if err != nil {
				t.Fatal(err)
			}

			edit, err := decoder.Decode(record)
			if err != nil {
				t.Fatal(err)
			}
			if testCase.wantDiff == "" {
				if edit != nil {
					t.Fatalf("invalid patch result produced edit: %+v", edit)
				}
			} else if edit == nil || edit.Filename != "a.go" || edit.Diff != testCase.wantDiff || !edit.Truncated {
				t.Fatalf("multi-file patch = %+v", edit)
			}
			if len(decoder.pending) != 0 {
				t.Fatal("completed request remained pending")
			}
			if duplicate, err := decoder.Decode(record); err != nil || duplicate != nil {
				t.Fatalf("duplicate result = %+v, error = %v", duplicate, err)
			}
		})
	}
}
