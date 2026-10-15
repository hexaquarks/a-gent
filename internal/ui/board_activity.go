package ui

import (
	"strings"
	"time"

	"a-gent/internal/agent"

	"github.com/charmbracelet/x/ansi"
)

const boardActivityFreshness = 5 * time.Second

func (model Model) boardActivitySummary(session agent.Session, now time.Time) string {
	switch sessionState(session) {
	case agent.StateUnavailable:
		return "Status unavailable"
	case agent.StateError:
		return "Session error"
	case agent.StateWaiting:
		return "Waiting for input"
	case agent.StateIdle:
		if model.finishedThisVisit(session, now) {
			return "Finished this visit"
		}
		return "No current activity"
	case agent.StateRunning:
		// Public activity can refine a running session's summary below.
	default:
		return "Status unavailable"
	}

	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	cached := model.previewCache[identity]
	cacheAge := now.Sub(cached.checked)
	if model.previewKey != identity || !cached.loaded || cached.err != nil || cached.activity == nil ||
		cacheAge < 0 || cacheAge > boardActivityFreshness {
		return "Working…"
	}

	// Only the selected session is refreshed. Old public output must not look
	// like a tool operation that is still in progress.
	label := currentActivityLabel(*cached.activity, now)

	text := ""
	for _, line := range strings.Split(ansi.Strip(cached.activity.Text), "\n") {
		text = strings.TrimSpace(safeDisplayText(line))
		if text != "" {
			break
		}
	}

	switch {
	case text == "" && label == "":
		return "Working…"
	case text == "":
		return label
	case label == "":
		return text
	default:
		return label + " · " + text
	}
}

// A fresh cache read can still contain an old completed action. Keep its output
// visible without presenting that action as work the agent is doing now.
func currentActivityLabel(activity agent.Activity, now time.Time) string {
	age := now.Sub(activity.At)
	if activity.At.IsZero() || age < 0 || age > boardActivityFreshness {
		return "Last output"
	}
	return strings.TrimSpace(safeDisplayText(activity.Label))
}
