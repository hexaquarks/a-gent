package ui

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const readyPulseDuration = 1250 * time.Millisecond

type readyPulseMessage time.Time

func readyPulseTimer() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(now time.Time) tea.Msg {
		return readyPulseMessage(now)
	})
}

func (model *Model) updateReadyPulses(sessions []agent.Session, now time.Time) {
	previous := make(map[sessionIdentity]agent.State, len(model.sessions))
	for _, session := range model.sessions {
		previous[sessionIdentity{provider: session.Provider, id: session.ID}] = session.State
	}

	pulses := make(map[sessionIdentity]time.Time)
	for _, session := range sessions {
		identity := sessionIdentity{provider: session.Provider, id: session.ID}
		if session.Stale || (session.State != agent.StateIdle && session.State != agent.StateWaiting) {
			continue
		}
		if previous[identity] == agent.StateRunning {
			pulses[identity] = now
		} else if started, ok := model.readyPulses[identity]; ok && now.Sub(started) < readyPulseDuration {
			pulses[identity] = started
		}
	}
	model.readyPulses = pulses
	model.readyPulseTime = now
}

func (model Model) readyPulseBackground(session agent.Session, background lipgloss.Color) lipgloss.Color {
	started, ok := model.readyPulses[sessionIdentity{provider: session.Provider, id: session.ID}]
	if !ok {
		return background
	}
	progress := float64(model.readyPulseTime.Sub(started)) / float64(readyPulseDuration)
	if progress <= 0 || progress >= 1 {
		return background
	}

	base := background
	if base == "" {
		base = lipgloss.Color(colorBackground)
	}
	baseRGB, _ := strconv.ParseUint(string(base)[1:], 16, 32)
	accentRGB, _ := strconv.ParseUint(colorAccent[1:], 16, 32)
	// A single rise and fall keeps text readable and returns selected
	// rows to their normal highlight without changing the cursor or unread dot.
	strength := 0.45 * math.Sin(math.Pi*progress)
	var channels [3]uint8
	for index, shift := range []uint{16, 8, 0} {
		from := float64((baseRGB >> shift) & 0xff)
		to := float64((accentRGB >> shift) & 0xff)
		channels[index] = uint8(math.Round(from + (to-from)*strength))
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", channels[0], channels[1], channels[2]))
}
