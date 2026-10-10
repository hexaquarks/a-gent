package ui

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
)

// Waits for the next provider result without blocking keyboard input.
// The UI starts another wait after handling that result.
func awaitProviderUpdate(updates <-chan polling.Update) tea.Cmd {
	if updates == nil {
		return nil
	}
	return func() tea.Msg {
		update, ok := <-updates
		if !ok {
			return nil
		}
		return update
	}
}

// Updates only the provider that reported a result, leaving the others alone.
// If it failed, keeps its previous sessions but marks their status as out of date.
func (model *Model) applyProviderUpdate(update polling.Update) {
	sessions := make([]agent.Session, 0, len(model.sessions)+len(update.Sessions))
	for _, session := range model.sessions {
		if session.Provider != update.Provider {
			sessions = append(sessions, session)
		} else if update.Err != nil {
			session.Stale = true
			sessions = append(sessions, session)
		}
	}

	if update.Err != nil {
		model.providerErrors[update.Provider] = update.Err
	} else {
		delete(model.providerErrors, update.Provider)
		for _, session := range update.Sessions {
			session.Provider = update.Provider
			session.Stale = false
			sessions = append(sessions, session)
		}
	}

	model.updateUnreadSessions(sessions)
	model.updateReadyPulses(sessions, time.Now())
	model.sessions = sessions
	model.clearMissingProjectFilter()

	var providerErrors []error
	for _, provider := range model.failedProviders() {
		providerErrors = append(providerErrors, fmt.Errorf("%s: %w", provider, model.providerErrors[provider]))
	}
	model.lastError = errors.Join(providerErrors...)
	model.updateTableRows()
}

func (model Model) failedProviders() []string {
	providers := make([]string, 0, len(model.providerErrors))
	for provider := range model.providerErrors {
		providers = append(providers, provider)
	}
	slices.Sort(providers)
	return providers
}

// Shows out-of-date sessions as unavailable without changing their saved status.
// Keeping that status lets us notice completed work when the provider returns.
func sessionState(session agent.Session) agent.State {
	if session.Stale {
		return agent.StateUnavailable
	}
	return session.State
}
