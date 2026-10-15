package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const inspectorActivityMaxTextRows = 3

// Compact previews keep link labels readable; the expanded view retains the
// provider's complete output, including URLs and Markdown formatting.
var inspectorInlineLink = regexp.MustCompile(`\[([^\]\n]+)\]\(https?://[^\s)]+\)`)

func inspectorActivityExcerpt(text string, width, rows int) []string {
	if rows <= 0 {
		return nil
	}
	text = inspectorInlineLink.ReplaceAllString(text, "$1")
	text = strings.ReplaceAll(text, "`", "")
	sourceLines := strings.Split(text, "\n")
	for index, line := range sourceLines {
		sourceLines[index] = safeDisplayText(strings.ReplaceAll(line, "\t", "  "))
	}
	text = strings.Join(sourceLines, "\n")
	lines := strings.Split(ansi.Wrap(strings.TrimSpace(text), width, ""), "\n")
	if len(lines) > rows {
		lines = lines[:rows]
		lines[rows-1] = ansi.Truncate(strings.TrimRight(lines[rows-1], " ")+" …", width, "…")
	}
	return lines
}
