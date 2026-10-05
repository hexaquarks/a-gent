package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Provider text may contain terminal commands or newlines. Remove them before
// styling the display, keeping the original session data for navigation.
func safeDisplayText(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, ansi.Strip(value))
}
