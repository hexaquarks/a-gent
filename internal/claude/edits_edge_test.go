package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

func TestClaudeDecoderRejectsInvalidRecords(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		record    string
		wantError string
	}{
		{name: "malformed JSON", record: `{`, wantError: "unexpected end"},
		{name: "foreign session", record: `{"sessionId":"other"}`, wantError: "identity mismatch"},
		{name: "sidechain", record: `{"isSidechain":true,"message":{"content":[{"type":"text","text":"worker message"}]}}`},
		{name: "plain user message", record: `{"type":"user","message":{"content":"user text"}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			decoder := &editDecoder{sessionID: "one", pending: make(map[string]string)}
			edit, err := decoder.Decode([]byte(testCase.record))
			if testCase.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("error = %v, want %q", err, testCase.wantError)
			}
			if edit != nil || decoder.Activity() != nil || len(decoder.pending) != 0 {
				t.Fatalf("invalid record changed preview: edit = %+v, activity = %+v", edit, decoder.Activity())
			}
		})
	}
}

func TestClaudeFailedToolResultReplacesPublicActivity(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: map[string]string{"edit": "Edit"}}
	record := `{"type":"user","sessionId":"one","message":{"content":[{"type":"tool_result","tool_use_id":"edit","is_error":true,"content":"Permission denied"}]}}`
	edit, err := decoder.Decode([]byte(record))
	activity := decoder.Activity()
	if err != nil || edit != nil || activity == nil || activity.Label != "Tool failed" || activity.Text != "Permission denied" {
		t.Fatalf("edit = %+v, activity = %+v, error = %v", edit, activity, err)
	}
	if len(decoder.pending) != 0 {
		t.Fatal("failed request remained pending")
	}
}

func TestClaudeUnchangedWriteDoesNotProduceAnEdit(t *testing.T) {
	decoder := &editDecoder{sessionID: "one", pending: map[string]string{"write": "Write"}}
	record := `{"type":"user","sessionId":"one","message":{"content":[{"type":"tool_result","tool_use_id":"write"}]},"toolUseResult":{"type":"update","filePath":"same.go","content":"unchanged","originalFile":"unchanged","structuredPatch":[]}}`
	edit, err := decoder.Decode([]byte(record))
	if err != nil || edit != nil || len(decoder.pending) != 0 {
		t.Fatalf("no-op write = %+v, error = %v, pending = %+v", edit, err, decoder.pending)
	}
}

func TestClaudePreviewValidatesTranscriptIdentity(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, testCase := range []struct {
		name    string
		adapter Adapter
		session agent.Session
	}{
		{name: "invalid transcript ID", adapter: NewAdapter(), session: agent.Session{ID: "job:one"}},
		{name: "path traversal", adapter: NewAdapter(), session: agent.Session{ID: "one", TranscriptID: "../other"}},
		{name: "uninitialized adapter", adapter: Adapter{}, session: agent.Session{ID: "one"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			preview, err := testCase.adapter.LatestPreview(context.Background(), testCase.session)
			if err == nil || !strings.Contains(err.Error(), "identity unavailable") || preview.Edit != nil || preview.Activity != nil {
				t.Fatalf("preview = %+v, error = %v", preview, err)
			}
		})
	}
}

func TestClaudePreviewUsesDefaultConfigDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	project := filepath.Join(home, ".claude", "projects", "fixture")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	record := `{"type":"assistant","sessionId":"one","message":{"content":[{"type":"text","text":"From isolated home"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(project, "one.jsonl"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}

	preview, err := NewAdapter().LatestPreview(context.Background(), agent.Session{ID: "one"})
	if err != nil || preview.Activity == nil || preview.Activity.Text != "From isolated home" || preview.Edit != nil {
		t.Fatalf("preview = %+v, error = %v", preview, err)
	}
}

func TestClaudePreviewRejectsAmbiguousTranscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	for _, projectName := range []string{"first", "second"} {
		project := filepath.Join(root, "projects", projectName)
		if err := os.MkdirAll(project, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "one.jsonl"), []byte("unreadable as JSON"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	preview, err := NewAdapter().LatestPreview(context.Background(), agent.Session{ID: "one"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || preview.Edit != nil || preview.Activity != nil {
		t.Fatalf("preview = %+v, error = %v", preview, err)
	}
}
