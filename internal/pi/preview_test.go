package pi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTranscript(t *testing.T, record *runtimeRecord, entries ...any) {
	t.Helper()
	file, err := os.Create(record.SessionFile)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.Encode(map[string]any{"type": "session", "version": 3, "id": record.SessionID, "cwd": record.Directory})
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			t.Fatal(err)
		}
	}
}

func message(id, parent, role string, content ...map[string]any) map[string]any {
	return map[string]any{"type": "message", "id": id, "parentId": parent,
		"timestamp": time.Unix(1999, 0).UTC().Format(time.RFC3339Nano),
		"message":   map[string]any{"role": role, "content": content}}
}

func textPart(text string) map[string]any { return map[string]any{"type": "text", "text": text} }
func editCall(id, path string) map[string]any {
	return map[string]any{"type": "toolCall", "id": id, "name": "edit", "arguments": map[string]any{"path": path}}
}
func editResult(id, parent, call string, failed bool) map[string]any {
	entry := message(id, parent, "toolResult", textPart("Edit completed"))
	body := entry["message"].(map[string]any)
	body["toolCallId"] = call
	body["toolName"] = "edit"
	body["isError"] = failed
	body["details"] = map[string]any{"diff": "-1 old\n+1 verified change"}
	return entry
}

func TestPreviewFollowsActiveBranchAndCompletedNativeEdits(t *testing.T) {
	adapter, record, _ := fixtureAdapter(t)
	record.SessionFile = filepath.Join(t.TempDir(), "session.jsonl")
	record.LeafID = "current"
	writeTranscript(t, record,
		map[string]any{"type": "message", "id": "system", "parentId": nil,
			"message": map[string]any{"role": "system", "content": "PRIVATE_SYSTEM"}},
		message("request", "system", "assistant", editCall("call-one", "file.go")),
		editResult("result", "request", "call-one", false),
		message("abandoned", "result", "assistant", textPart("ABANDONED_MUST_STAY_HIDDEN")),
		message("current", "result", "assistant", textPart("Public output"),
			map[string]any{"type": "thinking", "thinking": "PRIVATE_MUST_STAY_HIDDEN"}),
	)
	writeRuntime(t, adapter, record)
	sessions, _ := adapter.Sessions(context.Background())
	preview, err := adapter.LatestPreview(context.Background(), sessions[0])
	if err != nil || preview.Activity == nil || preview.Activity.Text != "Public output" ||
		preview.Edit == nil || preview.Edit.Filename != "file.go" || !strings.Contains(preview.Edit.Diff, "verified change") {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	// Moving the leaf without appending a transcript must invalidate the cache.
	record.LeafID = "request"
	writeRuntime(t, adapter, record)
	preview, err = adapter.LatestPreview(context.Background(), sessions[0])
	if err != nil || preview.Edit != nil || preview.Activity != nil {
		t.Fatalf("cached branch leaked output: %+v, %v", preview, err)
	}
	// Tree navigation must remove the retained edit if the new branch never made it.
	record.LeafID = "other"
	file, _ := os.OpenFile(record.SessionFile, os.O_APPEND|os.O_WRONLY, 0600)
	json.NewEncoder(file).Encode(message("other", "", "assistant", textPart("Other branch")))
	file.Close()
	writeRuntime(t, adapter, record)
	preview, err = adapter.LatestPreview(context.Background(), sessions[0])
	if err != nil || preview.Edit != nil || preview.Activity.Text != "Other branch" {
		t.Fatalf("branch preview: %+v, %v", preview, err)
	}
}

func TestPreviewDoesNotTreatRequestsFailedOrUnmatchedResultsAsEdits(t *testing.T) {
	for _, kind := range []string{"request", "failed", "unmatched", "wrong-tool", "write"} {
		t.Run(kind, func(t *testing.T) {
			adapter, record, _ := fixtureAdapter(t)
			record.SessionFile = filepath.Join(t.TempDir(), "session.jsonl")
			request := message("request", "", "assistant", editCall("call-one", "file.go"))
			result := editResult("result", "request", "call-one", kind == "failed")
			body := result["message"].(map[string]any)
			switch kind {
			case "unmatched":
				body["toolCallId"] = "another-call"
			case "wrong-tool":
				body["toolName"] = "write"
			case "write":
				request["message"].(map[string]any)["content"].([]map[string]any)[0]["name"] = "write"
				body["toolName"] = "write"
			}
			record.LeafID = "result"
			if kind == "request" {
				record.LeafID = "request"
				writeTranscript(t, record, request)
			} else {
				writeTranscript(t, record, request, result)
			}
			writeRuntime(t, adapter, record)
			sessions, _ := adapter.Sessions(context.Background())
			preview, err := adapter.LatestPreview(context.Background(), sessions[0])
			if err != nil || preview.Edit != nil {
				t.Fatalf("unverified edit: %+v, %v", preview, err)
			}
		})
	}
}

func TestPreviewRejectsChangedSessionForeignHeaderAndInvalidBranch(t *testing.T) {
	for _, kind := range []string{"session-change", "header", "cycle", "missing-parent", "exit", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			adapter, record, instances := fixtureAdapter(t)
			record.SessionFile = filepath.Join(t.TempDir(), "session.jsonl")
			record.LeafID = "leaf"
			writeTranscript(t, record, message("leaf", "", "assistant", textPart("Public")))
			writeRuntime(t, adapter, record)
			sessions, _ := adapter.Sessions(context.Background())
			ctx := context.Background()
			switch kind {
			case "session-change":
				record.SessionID = "another-session"
				writeRuntime(t, adapter, record)
			case "header":
				record.SessionID = "foreign"
				writeTranscript(t, record)
			case "cycle":
				writeTranscript(t, record, message("leaf", "leaf", "assistant", textPart("Private branch")))
			case "missing-parent":
				writeTranscript(t, record, message("leaf", "missing", "assistant", textPart("Partial branch")))
			case "exit":
				*instances = nil
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := adapter.LatestPreview(ctx, sessions[0]); err == nil {
				t.Fatal("invalid session preview accepted")
			}
		})
	}
}

func TestPreviewCacheDoesNotReuseAFileForAnotherSession(t *testing.T) {
	adapter, record, _ := fixtureAdapter(t)
	record.SessionFile = filepath.Join(t.TempDir(), "session.jsonl")
	record.LeafID = "leaf"
	writeTranscript(t, record, message("leaf", "", "assistant", textPart("Session one only")))
	writeRuntime(t, adapter, record)
	sessions, _ := adapter.Sessions(context.Background())
	if _, err := adapter.LatestPreview(context.Background(), sessions[0]); err != nil {
		t.Fatal(err)
	}
	record.SessionID = "another-session"
	writeRuntime(t, adapter, record)
	sessions, _ = adapter.Sessions(context.Background())
	if _, err := adapter.LatestPreview(context.Background(), sessions[0]); err == nil {
		t.Fatal("cache leaked the previous session's output")
	}
}
