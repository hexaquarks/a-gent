package editpreview

import (
	"strings"
	"time"
	"unicode/utf8"

	"a-gent/internal/agent"
)

// PublicActivity bounds retained output while preserving the newest text.
func PublicActivity(label, text string, at time.Time) *agent.Activity {
	const limit = 8192
	text = strings.TrimSpace(text)
	if len(text) > limit {
		start := len(text) - limit
		for !utf8.RuneStart(text[start]) {
			start++
		}
		text = "… truncated\n" + text[start:]
	}
	return &agent.Activity{Label: label, Text: text, At: at}
}
