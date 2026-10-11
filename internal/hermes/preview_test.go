package hermes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

func writePublicPreview(t *testing.T, adapter Adapter, value map[string]any) {
	t.Helper()
	directory := filepath.Join(adapter.directory, "previews")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(value)
	if err := os.WriteFile(filepath.Join(directory, "42.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewReadsOnlyMatchingLiveSession(t *testing.T) {
	adapter, record, instances := fixtureAdapter(t)
	sessions, _ := adapter.Sessions(context.Background())
	value := map[string]any{"sessionID": record.SessionID, "directory": record.Directory,
		"activity":  map[string]any{"label": "Assistant", "text": "public output", "at": 1999000},
		"edit":      map[string]any{"filename": "file.go", "diff": "-old\n+new", "at": 1998000},
		"reasoning": "PRIVATE"}
	writePublicPreview(t, adapter, value)
	preview, err := adapter.LatestPreview(context.Background(), sessions[0])
	if err != nil || preview.Activity.Text != "public output" || preview.Edit.Filename != "file.go" {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	for _, mismatch := range []string{"sessionID", "directory"} {
		original := value[mismatch]
		value[mismatch] = "wrong"
		writePublicPreview(t, adapter, value)
		if _, err := adapter.LatestPreview(context.Background(), sessions[0]); err == nil {
			t.Fatal("accepted wrong " + mismatch)
		}
		value[mismatch] = original
	}
	writePublicPreview(t, adapter, value)
	record.SessionID = "new-session"
	writeRuntime(t, adapter, record)
	if _, err := adapter.LatestPreview(context.Background(), sessions[0]); err == nil {
		t.Fatal("read switched session")
	}
	*instances = nil
	if _, err := adapter.LatestPreview(context.Background(), sessions[0]); err == nil {
		t.Fatal("read dead process")
	}
}

func TestPreviewBoundsOutputAndRejectsStaleMetadata(t *testing.T) {
	adapter, record, _ := fixtureAdapter(t)
	sessions, _ := adapter.Sessions(context.Background())
	writePublicPreview(t, adapter, map[string]any{"sessionID": record.SessionID, "directory": record.Directory,
		"activity": map[string]any{"text": strings.Repeat("x", 10000)},
		"edit":     map[string]any{"filename": "file", "diff": strings.Repeat("+", 70000)}})
	preview, err := adapter.LatestPreview(context.Background(), sessions[0])
	if err != nil || len(preview.Activity.Text) > 8250 || len(preview.Edit.Diff) > 65536 || !preview.Edit.Truncated {
		t.Fatalf("unbounded preview: %v", err)
	}
	record.UpdatedAt -= 6000
	writeRuntime(t, adapter, record)
	if _, err := adapter.LatestPreview(context.Background(), sessions[0]); err == nil {
		t.Fatal("read stale preview")
	}
	if _, err := adapter.LatestPreview(context.Background(), agent.Session{Provider: "pi"}); err == nil {
		t.Fatal("accepted other provider")
	}
}

func TestPreviewShortensNativePatchHeadersInsideProject(t *testing.T) {
	adapter, record, _ := fixtureAdapter(t)
	sessions, _ := adapter.Sessions(context.Background())
	writePublicPreview(t, adapter, map[string]any{"sessionID": record.SessionID, "directory": record.Directory,
		"edit": map[string]any{"filename": "/project/fixture.txt",
			"diff": "--- a//project/fixture.txt\n+++ b//project/fixture.txt\n@@ -1 +1 @@\n-old\n+new\n"}})
	preview, err := adapter.LatestPreview(context.Background(), sessions[0])
	if err != nil || preview.Edit.Filename != "fixture.txt" ||
		preview.Edit.Diff != "--- a/fixture.txt\n+++ b/fixture.txt\n@@ -1 +1 @@\n-old\n+new\n" {
		t.Fatalf("native patch preview: %+v, %v", preview.Edit, err)
	}
}
