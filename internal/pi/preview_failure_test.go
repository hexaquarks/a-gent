package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

func TestPreviewRejectsMissingIdentityAndDiscoveryFailures(t *testing.T) {
	adapter, record, _ := fixtureAdapter(t)
	record.SessionFile = filepath.Join(t.TempDir(), "session.jsonl")
	writeRuntime(t, adapter, record)
	sessions, err := adapter.Sessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	for _, session := range []agent.Session{
		{Provider: "other"},
		{Provider: "pi", TranscriptID: record.SessionID},
		{Provider: "pi", ProcessID: sessions[0].ProcessID},
	} {
		if _, err := adapter.LatestPreview(t.Context(), session); err == nil {
			t.Fatalf("accepted incomplete identity: %+v", session)
		}
	}

	discoveryError := errors.New("fixture discovery failed")
	adapter.discover = func(context.Context) ([]instance, error) { return nil, discoveryError }
	if _, err := adapter.LatestPreview(t.Context(), sessions[0]); !errors.Is(err, discoveryError) {
		t.Fatalf("discovery error = %v", err)
	}
}

func TestPreviewHandlesEmptyLeafAndUncachedReader(t *testing.T) {
	adapter, record, instances := fixtureAdapter(t)
	record.SessionFile = filepath.Join(t.TempDir(), "session.jsonl")
	writeRuntime(t, adapter, record)
	sessions, err := adapter.Sessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	preview, err := adapter.LatestPreview(t.Context(), sessions[0])
	if err != nil || preview.Activity != nil || preview.Edit != nil {
		t.Fatalf("empty active leaf = %+v, %v", preview, err)
	}

	record.LeafID = "leaf"
	writeTranscript(t, record, message("leaf", "", "assistant", textPart("Selected output")))
	writeRuntime(t, adapter, record)
	adapter.preview = nil
	*instances = append([]instance{{pid: 99}}, (*instances)...)
	preview, err = adapter.LatestPreview(t.Context(), sessions[0])
	if err != nil || preview.Activity == nil || preview.Activity.Text != "Selected output" {
		t.Fatalf("uncached selected read = %+v, %v", preview, err)
	}
}

func TestPreviewReaderRejectsMissingMalformedAndOversizedFiles(t *testing.T) {
	for _, kind := range []string{"missing", "malformed record", "malformed public content", "oversized", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			record := runtimeRecord{
				SessionID: "selected", Directory: "/fixture", LeafID: "leaf",
				SessionFile: filepath.Join(t.TempDir(), "session.jsonl"),
			}
			ctx := t.Context()
			want := ""
			switch kind {
			case "missing":
				// Leave the selected transcript absent.
			case "malformed record":
				if err := os.WriteFile(record.SessionFile, []byte("{broken\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				want = "decode Pi transcript"
			case "malformed public content":
				entry := message("leaf", "", "assistant")
				entry["message"].(map[string]any)["content"] = "not a content array"
				writeTranscript(t, &record, entry)
				want = "decode Pi public message"
			case "oversized":
				file, err := os.Create(record.SessionFile)
				if err != nil {
					t.Fatal(err)
				}
				const oversizedTranscriptBytes = 64*1024*1024 + 1
				err = file.Truncate(oversizedTranscriptBytes)
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("create oversized transcript: %v, %v", err, closeErr)
				}
				want = "exceeds 64 MiB"
			case "cancelled":
				writeTranscript(t, &record, message("leaf", "", "assistant", textPart("Output")))
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			preview, err := readPreview(ctx, record)
			if err == nil || preview.Activity != nil || preview.Edit != nil {
				t.Fatalf("invalid transcript returned %+v, %v", preview, err)
			}
			if want != "" && !strings.Contains(err.Error(), want) {
				t.Fatalf("read error = %v, want %q", err, want)
			}
			if kind == "missing" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing transcript error = %v", err)
			}
			if kind == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled read error = %v", err)
			}
		})
	}
}

func TestPreviewCacheReturnsUnchangedOutputAndRejectsDeletedFile(t *testing.T) {
	record := runtimeRecord{
		SessionID: "selected", Directory: "/fixture", LeafID: "leaf",
		SessionFile: filepath.Join(t.TempDir(), "session.jsonl"),
	}
	writeTranscript(t, &record, message("leaf", "", "assistant", textPart("Cached public output")))
	cache := &previewCache{}
	first, err := cache.read(t.Context(), record)
	if err != nil || first.Activity == nil {
		t.Fatalf("initial preview = %+v, %v", first, err)
	}
	second, err := cache.read(t.Context(), record)
	if err != nil || second.Activity == nil || second.Activity.Text != first.Activity.Text {
		t.Fatalf("unchanged transcript = %+v, %v", second, err)
	}
	if err := os.Remove(record.SessionFile); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.read(t.Context(), record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted transcript returned cached output: %v", err)
	}
}
