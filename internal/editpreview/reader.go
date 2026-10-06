// Package editpreview incrementally reads provider-owned session transcripts.
package editpreview

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"a-gent/internal/agent"
)

// Decoder retains request/result associations between appended transcript lines.
type Decoder interface {
	Decode([]byte) (*agent.Edit, error)
	Activity() *agent.Activity
}

type entry struct {
	path    string
	info    os.FileInfo
	offset  int64
	decoder Decoder
	latest  *agent.Edit
}

// Reader caches transcript offsets and decoders by session identity. Returned
// edits are immutable; the UI can safely retain them during background refreshes.
type Reader struct {
	mu      sync.Mutex
	entries map[string]*entry
}

// Read uses one transcript cursor for both public output and file edits.
func (reader *Reader) Read(ctx context.Context, key, path string, newDecoder func() Decoder) (agent.Preview, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		return agent.Preview{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return agent.Preview{}, err
	}
	if reader.entries == nil {
		reader.entries = make(map[string]*entry)
	}
	cached := reader.entries[key]
	if cached == nil || cached.path != path || !os.SameFile(cached.info, info) || info.Size() < cached.offset {
		cached = &entry{path: path, info: info, decoder: newDecoder()}
		reader.entries[key] = cached
	}
	if _, err := file.Seek(cached.offset, io.SeekStart); err != nil {
		return agent.Preview{}, err
	}
	// Limit a refresh to the size observed at open, even if the agent keeps writing.
	lines := bufio.NewReader(io.LimitReader(file, info.Size()-cached.offset))
	for {
		if err := ctx.Err(); err != nil {
			return agent.Preview{}, err
		}
		line, err := lines.ReadBytes('\n')
		if err == io.EOF {
			// Retry a partial final line on the next refresh.
			return agent.Preview{Edit: cached.latest, Activity: cached.decoder.Activity()}, nil
		}
		if err != nil {
			return agent.Preview{}, err
		}
		edit, err := cached.decoder.Decode(line)
		if err != nil {
			return agent.Preview{}, fmt.Errorf("decode session preview: %w", err)
		}
		cached.offset += int64(len(line))
		if edit != nil {
			// Keep cache size independent of the size of a full-file write.
			const limit = 64 * 1024
			if len(edit.Diff) > limit {
				end := limit
				for !utf8.RuneStart(edit.Diff[end]) {
					end--
				}
				// Copy the prefix so it does not retain the full write's backing string.
				edit.Diff = strings.Clone(edit.Diff[:end])
				edit.Truncated = true
			}
			cached.latest = edit
		}
	}
}
