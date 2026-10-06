package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

// Fixtures mirror the installed Claude Edit/Write result schemas, including
// staged edits that succeeded as tool calls but did not change the file.
func TestClaudeEditRequiresSuccessfulUnstagedResult(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		failed, staged bool
		toolID         string
		want           bool
	}{
		{name: "completed", toolID: "edit", want: true},
		{name: "failed", failed: true, toolID: "edit"},
		{name: "staged", staged: true, toolID: "edit"},
		{name: "unmatched", toolID: "other"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			decoder := &editDecoder{sessionID: "one", pending: map[string]string{}}
			edit, err := decoder.Decode([]byte(`{"type":"assistant","sessionId":"one","message":{"content":[{"type":"tool_use","id":"edit","name":"Edit","input":{"file_path":"requested.go","old_string":"old","new_string":"requested"}}]}}`))
			if err != nil || edit != nil {
				t.Fatal("request was presented as completed")
			}
			result := `{"type":"user","sessionId":"one","timestamp":"2026-10-05T12:00:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"TOOL","is_error":FAILED}]},"toolUseResult":{"filePath":"actual.go","staged":STAGED,"structuredPatch":[{"oldStart":2,"oldLines":2,"newStart":2,"newLines":2,"lines":[" context","-old","+actual"]}]}}`
			result = strings.ReplaceAll(result, "TOOL", scenario.toolID)
			result = strings.ReplaceAll(result, "FAILED", map[bool]string{true: "true", false: "false"}[scenario.failed])
			result = strings.ReplaceAll(result, "STAGED", map[bool]string{true: "true", false: "false"}[scenario.staged])
			edit, err = decoder.Decode([]byte(result))
			if err != nil || (edit != nil) != scenario.want {
				t.Fatalf("%#v %v", edit, err)
			}
			if edit != nil && (edit.Filename != "actual.go" || !strings.Contains(edit.Diff, "+actual") || edit.CompletedAt.IsZero()) {
				t.Fatal(edit)
			}
		})
	}
}

func TestClaudeAdapterReadsSessionSpecificWrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	project := filepath.Join(root, "projects", "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"type":"assistant","sessionId":"one","message":{"content":[{"type":"tool_use","id":"write","name":"Write"}]}}
{"type":"user","sessionId":"one","timestamp":"2026-10-05T12:00:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"write"}]},"toolUseResult":{"type":"create","filePath":"new.go","content":"hello\nworld\n","structuredPatch":[]}}
`
	if err := os.WriteFile(filepath.Join(project, "one.jsonl"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "unrelated.jsonl"), []byte("not valid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter()
	preview, err := adapter.LatestPreview(context.Background(), agent.Session{ID: "job:background", TranscriptID: "one"})
	edit := preview.Edit
	if err != nil || edit == nil || edit.Diff != "+hello\n+world" {
		t.Fatalf("%#v %v", edit, err)
	}
	if _, err := adapter.LatestPreview(context.Background(), agent.Session{ID: "missing"}); err == nil {
		t.Fatal("missing transcript presented as no edits")
	}
}

func TestWriteWithoutDiffDoesNotMisrepresentOlderEdit(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: map[string]string{"write": "Write"}}
	edit, err := decoder.Decode([]byte(`{"type":"user","sessionId":"one","message":{"content":[{"type":"tool_result","tool_use_id":"write"}]},"toolUseResult":{"type":"update","filePath":"large.go","content":"new","originalFile":null,"structuredPatch":[]}}`))
	if err != nil || edit == nil || edit.Diff != "" || edit.Filename != "large.go" {
		t.Fatalf("%#v %v", edit, err)
	}
}

func TestPublicActivityIgnoresThinkingAndTracksToolResults(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: make(map[string]string)}
	_, err := decoder.Decode([]byte(`{"type":"assistant","sessionId":"one","message":{"content":[{"type":"text","text":"Visible output"},{"type":"thinking","thinking":"private reasoning"}]}}`))
	if err != nil || decoder.Activity() == nil || decoder.Activity().Text != "Visible output" {
		t.Fatalf("%#v %v", decoder.Activity(), err)
	}
	_, err = decoder.Decode([]byte(`{"type":"user","sessionId":"one","message":{"content":[{"type":"tool_result","tool_use_id":"read","content":[{"type":"text","text":"Visible tool output"}]}]}}`))
	if err != nil || decoder.Activity().Text != "Visible tool output" {
		t.Fatalf("%#v %v", decoder.Activity(), err)
	}
}
