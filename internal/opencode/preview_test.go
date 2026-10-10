package opencode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a-gent/internal/agent"
)

func TestPreviewUsesSelectedPublicMessagesAndCompletedEdits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/ses_one/message" || r.URL.Query().Get("directory") != "/project" {
			t.Errorf("read another session: %s", r.URL)
		}
		w.Write([]byte(`[
   {"info":{"sessionID":"ses_one","role":"assistant","time":{"created":1000}},"parts":[
    {"sessionID":"ses_one","type":"text","text":"Public output","time":{"start":1100}},
    {"sessionID":"ses_one","type":"reasoning","text":"Private reasoning","time":{"start":9999}},
    {"sessionID":"ses_two","type":"text","text":"Other session","time":{"start":9999}},
    {"sessionID":"ses_one","type":"text","text":"Synthetic text","synthetic":true,"time":{"start":9999}},
    {"sessionID":"ses_one","type":"tool","tool":"edit","state":{"status":"completed","output":"Edit applied","time":{"end":1200},"metadata":{"diff":"@@ -1 +1 @@\n-old\n+new","filediff":{"file":"/project/main.go"}}}},
    {"sessionID":"ses_one","type":"tool","tool":"edit","state":{"status":"error","output":"Failed","time":{"end":9999},"metadata":{"diff":"bad","filepath":"bad.go"}}},
    {"sessionID":"ses_one","type":"tool","tool":"edit","state":{"status":"running","output":"Pending","time":{"end":9999},"metadata":{"diff":"bad","filepath":"pending.go"}}}
   ]},
   {"info":{"sessionID":"ses_two","role":"assistant","time":{"created":9999}},"parts":[{"sessionID":"ses_two","type":"text","text":"Other conversation"}]},
   {"info":{"sessionID":"ses_one","role":"user","time":{"created":9999}},"parts":[{"sessionID":"ses_one","type":"text","text":"User message"}]}
  ]`))
	}))
	defer server.Close()
	adapter := NewAdapter()
	session := agent.Session{ID: "process:1:0:ses_one", Provider: "opencode", TranscriptID: "ses_one", TranscriptPath: server.URL, WorkingDirectory: "/project"}
	preview, err := adapter.LatestPreview(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Activity == nil || preview.Activity.Text != "Edit applied" || preview.Activity.At.UnixMilli() != 1200 {
		t.Fatalf("activity: %+v", preview.Activity)
	}
	if preview.Edit == nil || preview.Edit.Filename != "/project/main.go" || !strings.Contains(preview.Edit.Diff, "+new") {
		t.Fatalf("edit: %+v", preview.Edit)
	}
	session.TranscriptID = "../ses_two"
	if _, err := adapter.LatestPreview(context.Background(), session); err == nil {
		t.Fatal("accepted invalid session identity")
	}
}
