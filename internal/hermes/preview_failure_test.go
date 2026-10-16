package hermes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewRejectsMissingAndMalformedSnapshots(t *testing.T) {
	for _, kind := range []string{"missing", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			adapter, _, _ := fixtureAdapter(t)
			sessions, err := adapter.Sessions(t.Context())
			if err != nil || len(sessions) != 1 {
				t.Fatalf("sessions = %+v, %v", sessions, err)
			}
			if kind == "malformed" {
				path := filepath.Join(adapter.directory, "previews")
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "42.json"), []byte("{broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			preview, err := adapter.LatestPreview(t.Context(), sessions[0])
			if err == nil || preview.Activity != nil || preview.Edit != nil {
				t.Fatalf("invalid snapshot = %+v, %v", preview, err)
			}
			if kind == "missing" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing snapshot error = %v", err)
			}
			if kind == "malformed" && !strings.Contains(err.Error(), "decode Hermes public preview") {
				t.Fatalf("malformed snapshot error = %v", err)
			}
		})
	}
}

func TestPreviewPropagatesDiscoveryFailureAndSkipsUnrelatedProcesses(t *testing.T) {
	adapter, record, instances := fixtureAdapter(t)
	sessions, err := adapter.Sessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	writePublicPreview(t, adapter, map[string]any{
		"sessionID": record.SessionID, "directory": record.Directory,
		"activity": map[string]any{"label": "Assistant", "text": "Selected output"},
	})
	*instances = append([]instance{{pid: 99}}, (*instances)...)
	preview, err := adapter.LatestPreview(t.Context(), sessions[0])
	if err != nil || preview.Activity == nil || preview.Activity.Text != "Selected output" {
		t.Fatalf("selected process preview = %+v, %v", preview, err)
	}

	discoveryError := errors.New("fixture discovery failed")
	adapter.discover = func(context.Context) ([]instance, error) { return nil, discoveryError }
	if _, err := adapter.LatestPreview(t.Context(), sessions[0]); !errors.Is(err, discoveryError) {
		t.Fatalf("discovery error = %v", err)
	}
}
