package agent

import "testing"

func TestStatesHaveDistinctValues(t *testing.T) {
	states := []State{
		StateRunning,
		StateWaiting,
		StateIdle,
		StateError,
		StateUnavailable,
	}
	seenStates := make(map[State]struct{}, len(states))

	for _, state := range states {
		if _, seen := seenStates[state]; seen {
			t.Fatalf("state %q is duplicated", state)
		}

		seenStates[state] = struct{}{}
	}
}
