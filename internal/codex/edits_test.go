package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
)

func TestCompletedFileChanges(t *testing.T) {
	for _, status := range []string{"completed", "failed", "declined", "inProgress"} {
		decoder := &editDecoder{sessionID: "one", pending: map[string]string{}}
		line := `{"type":"event_msg","timestamp":"2026-10-05T12:00:00Z","payload":{"type":"item_completed","thread_id":"one","completed_at_ms":1791201600000,"item":{"type":"FileChange","status":"` + status + `","changes":{"a.go":{"type":"update","unified_diff":"@@ -1 +1 @@\n-old\n+new"}}}}}`
		edit, err := decoder.Decode([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if (edit != nil) != (status == "completed") {
			t.Fatalf("status %s: %#v", status, edit)
		}
		if edit != nil && (edit.Filename != "a.go" || !strings.Contains(edit.Diff, "+new") || edit.CompletedAt.IsZero()) {
			t.Fatal(edit)
		}
		edit, err = decoder.Decode([]byte(strings.Replace(line, `"thread_id":"one"`, `"thread_id":"another"`, 1)))
		if err != nil || edit != nil {
			t.Fatal("accepted another session")
		}
	}
}

func TestPatchRequiresMatchingSuccessfulResult(t *testing.T) {
	for _, output := range []string{`{"output":"Success. Updated the following files:\nM a.go\n","metadata":{"exit_code":0}}`, `{"output":"Failed","metadata":{"exit_code":1}}`, `{"output":"Success. Updated the following files:\nM a.go\n"}`} {
		decoder := &editDecoder{sessionID: "one", pending: map[string]string{}}
		request := `{"type":"response_item","payload":{"type":"custom_tool_call","status":"completed","call_id":"patch","name":"apply_patch","input":"*** Begin Patch\n*** Update File: a.go\n@@\n-old\n+new\n*** End Patch"}}`
		edit, err := decoder.Decode([]byte(request))
		if err != nil || edit != nil {
			t.Fatal("request treated as success")
		}
		result := map[string]any{"type": "response_item", "timestamp": "2026-10-05T12:00:00Z", "payload": map[string]any{"type": "custom_tool_call_output", "call_id": "unrelated", "output": output}}
		data, _ := json.Marshal(result)
		edit, err = decoder.Decode(data)
		if err != nil || edit != nil {
			t.Fatal("unrelated result matched")
		}
		result["payload"].(map[string]any)["call_id"] = "patch"
		data, _ = json.Marshal(result)
		edit, err = decoder.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if (edit != nil) != strings.Contains(output, `"exit_code":0`) {
			t.Fatalf("output %s: %#v", output, edit)
		}
	}
}

func TestCodexAdapterReadsCompletedLocalRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	data := `{"type":"session_meta","payload":{"id":"one"}}
{"type":"event_msg","timestamp":"2026-10-05T12:00:00Z","payload":{"type":"item_completed","thread_id":"one","item":{"type":"FileChange","status":"completed","changes":{"new.go":{"type":"add","content":"hello\nworld\n"}}}}}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter()
	preview, err := adapter.LatestPreview(context.Background(), agent.Session{ID: "one", TranscriptPath: path})
	edit := preview.Edit
	if err != nil || edit == nil || edit.Diff != "+hello\n+world" || !edit.CompletedAt.Equal(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("%#v %v", edit, err)
	}
}

func TestPublicActivityIgnoresReasoningAndOtherSessions(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: make(map[string]string)}
	records := []string{
		`{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Visible output"}]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"private reasoning"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"analysis","content":[{"type":"output_text","text":"private analysis"}]}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","thread_id":"other","item":{"type":"AgentMessage","content":[{"type":"Text","text":"another session"}]}}}`,
	}
	for _, record := range records {
		if _, err := decoder.Decode([]byte(record)); err != nil {
			t.Fatal(err)
		}
		if decoder.Activity() == nil || decoder.Activity().Text != "Visible output" {
			t.Fatalf("unexpected activity: %#v", decoder.Activity())
		}
	}
	_, err := decoder.Decode([]byte(`{"type":"event_msg","payload":{"type":"item_completed","thread_id":"one","item":{"type":"CommandExecution","command":["go","test"],"status":"completed","aggregated_output":"PASS"}}}`))
	if err != nil || !strings.Contains(decoder.Activity().Text, "PASS") {
		t.Fatal("command output did not replace public message")
	}
}
