package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// lastActiveStyle fades older activity while leaving unknown timestamps neutral.
func lastActiveStyle(lastActiveAt, now time.Time) lipgloss.Style {
	if lastActiveAt.IsZero() {
		return mutedStyle
	}

	elapsed := now.Sub(lastActiveAt)
	switch {
	case elapsed < time.Minute:
		return runningStyle
	case elapsed < 5*time.Minute:
		return shortcutKeyStyle
	case elapsed < time.Hour:
		return waitingStyle
	default:
		return mutedStyle
	}
}

func formatLastActiveAt(lastActiveAt, now time.Time) string {
	if lastActiveAt.IsZero() {
		return "—"
	}

	// Clamp future timestamps when the provider's clock is ahead of ours.
	elapsed := max(time.Duration(0), now.Sub(lastActiveAt))
	if elapsed < time.Minute {
		return fmt.Sprintf("%ds ago", int64(elapsed/time.Second))
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%dm ago", int64(elapsed/time.Minute))
	}

	return fmt.Sprintf("%dh ago", int64(elapsed/time.Hour))
}
