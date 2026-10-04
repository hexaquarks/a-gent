package ui

import "a-gent/internal/agent"

func (model *Model) updateUnreadSessions(sessions []agent.Session) {
	previousStates := make(map[sessionIdentity]agent.State, len(model.sessions))
	for _, session := range model.sessions {
		previousStates[sessionIdentity{provider: session.Provider, id: session.ID}] = session.State
	}

	unread := make(map[sessionIdentity]bool)
	for _, session := range sessions {
		identity := sessionIdentity{provider: session.Provider, id: session.ID}
		if session.State != agent.StateRunning &&
			(model.unreadSessions[identity] || previousStates[identity] == agent.StateRunning) {
			unread[identity] = true
		}
	}
	model.unreadSessions = unread
}

func (model *Model) markSelectedSessionRead() {
	if session, ok := model.selectedSession(); ok {
		delete(model.unreadSessions, sessionIdentity{provider: session.Provider, id: session.ID})
	}
}
