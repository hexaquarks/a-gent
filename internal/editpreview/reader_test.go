package editpreview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"a-gent/internal/agent"
)

type countingDecoder struct{ calls int }

func (decoder *countingDecoder) Activity() *agent.Activity { return nil }

func (decoder *countingDecoder) Decode(line []byte) (*agent.Edit, error) {
	decoder.calls++
	return &agent.Edit{Filename: string(line), Diff: "+new"}, nil
}

type largeEditDecoder struct{ countingDecoder }

func (decoder *largeEditDecoder) Decode([]byte) (*agent.Edit, error) {
	return &agent.Edit{Filename: "large.go", Diff: strings.Repeat("界", 30000)}, nil
}

func TestReaderBoundsDiffAndResetsReplacedTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{}
	load := func() agent.Preview {
		t.Helper()
		preview, err := reader.Read(context.Background(), "one", path, func() Decoder { return &largeEditDecoder{} })
		if err != nil {
			t.Fatal(err)
		}
		return preview
	}
	edit := load().Edit
	if edit == nil || !edit.Truncated || len(edit.Diff) > 64*1024 || !utf8.ValidString(edit.Diff) {
		t.Fatal("large diff was not bounded at a valid character boundary")
	}
	if err := os.WriteFile(path+".new", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
	if preview := load(); preview.Edit != nil || preview.Activity != nil {
		t.Fatal("replacement transcript retained old records")
	}
}

func TestPublicActivityBoundsUnicodeOutput(t *testing.T) {
	activity := PublicActivity("Assistant", strings.Repeat("界", 10000)+"latest", time.Now())
	if len(activity.Text) > 8192+len("… truncated\n") || !utf8.ValidString(activity.Text) || !strings.HasSuffix(activity.Text, "latest") || !strings.Contains(activity.Text, "truncated") {
		t.Fatal("large output lost its size bound, character boundary, or newest text")
	}
}

func TestReaderOnlyLoadsAppendedCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("first\npartial"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{}
	decoder := &countingDecoder{}
	load := func() *agent.Edit {
		t.Helper()
		preview, err := reader.Read(context.Background(), "codex:one", path, func() Decoder { return decoder })
		if err != nil {
			t.Fatal(err)
		}
		return preview.Edit
	}
	if edit := load(); edit.Filename != "first\n" {
		t.Fatal(edit)
	}
	load()
	if decoder.calls != 1 {
		t.Fatal("unchanged history reloaded")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(" completed\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if edit := load(); edit.Filename != "partial completed\n" || decoder.calls != 2 {
		t.Fatalf("%#v calls=%d", edit, decoder.calls)
	}
	if err := os.WriteFile(path, []byte("reset\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if edit := load(); edit.Filename != "reset\n" {
		t.Fatal("truncation not detected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Read(ctx, "codex:one", path, func() Decoder { return decoder }); err == nil {
		t.Fatal("cancellation ignored")
	}
}
