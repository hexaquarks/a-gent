package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

func TestPreviewReportsLocalAPIFailures(t *testing.T) {
	t.Setenv("OPENCODE_SERVER_PASSWORD", "")

	for _, test := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "missing session", status: http.StatusNotFound, want: "HTTP 404"},
		{name: "malformed response", status: http.StatusOK, body: "{broken", want: "Decode OpenCode"},
		{name: "null response", status: http.StatusOK, body: "null", want: "returned null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			session := agent.Session{Provider: "opencode", TranscriptID: "selected", TranscriptPath: server.URL}
			preview, err := NewAdapter().LatestPreview(t.Context(), session)
			if err == nil || !strings.Contains(err.Error(), test.want) || preview.Activity != nil || preview.Edit != nil {
				t.Fatalf("failed API preview = %+v, %v; want %q", preview, err, test.want)
			}
		})
	}
}

func TestPreviewFallsBackToMessageTimeToolTitleAndNativePatch(t *testing.T) {
	t.Setenv("OPENCODE_SERVER_PASSWORD", "")

	const diffLimit = 64 * 1024
	patch := strings.Repeat("+", diffLimit+1)
	message := map[string]any{
		"info": map[string]any{"sessionID": "selected", "role": "assistant", "time": map[string]any{"created": 2000}},
		"parts": []map[string]any{
			{"sessionID": "selected", "type": "text", "text": "Newest output"},
			{
				"sessionID": "selected", "type": "tool", "tool": "edit",
				"state": map[string]any{
					"status": "completed", "title": "Edit completed", "time": map[string]any{"end": 1000},
					"metadata": map[string]any{"filediff": map[string]any{"file": "file.go", "patch": patch}},
				},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode([]any{message}); err != nil {
			t.Errorf("encode preview response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	session := agent.Session{Provider: "opencode", TranscriptID: "selected", TranscriptPath: server.URL}
	preview, err := NewAdapter().LatestPreview(t.Context(), session)
	if err != nil || preview.Activity == nil || preview.Activity.Text != "Newest output" ||
		preview.Activity.At.UnixMilli() != 2000 {
		t.Fatalf("older tool replaced newer assistant output: %+v, %v", preview, err)
	}
	if preview.Edit == nil || preview.Edit.Filename != "file.go" || len(preview.Edit.Diff) != diffLimit ||
		!preview.Edit.Truncated || preview.Edit.CompletedAt.UnixMilli() != 1000 {
		t.Fatalf("native patch fallback was not bounded: %+v", preview.Edit)
	}
}

func TestPreviewUsesCompletedToolTitleWhenOutputIsEmpty(t *testing.T) {
	t.Setenv("OPENCODE_SERVER_PASSWORD", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"info":{"sessionID":"selected","role":"assistant"},"parts":[
			{"sessionID":"selected","type":"tool","tool":"bash","state":{
				"status":"completed","title":"Tests passed","time":{"end":1000}}}]}]`))
	}))
	t.Cleanup(server.Close)
	session := agent.Session{Provider: "opencode", TranscriptID: "selected", TranscriptPath: server.URL}
	preview, err := NewAdapter().LatestPreview(t.Context(), session)
	if err != nil || preview.Activity == nil || preview.Activity.Label != "bash" ||
		preview.Activity.Text != "Tests passed" || preview.Edit != nil {
		t.Fatalf("tool title fallback = %+v, %v", preview, err)
	}
}
