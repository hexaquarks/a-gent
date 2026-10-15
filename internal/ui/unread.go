package ui

import (
	"time"

	"a-gent/internal/agent"
)

const finishedHintDuration = 5 * time.Minute

func (model *Model) updateUnreadSessions(sessions []agent.Session) {
	model.updateUnreadSessionsAt(sessions, time.Now())
}

func (model *Model) updateUnreadSessionsAt(sessions []agent.Session, now time.Time) {
	previousStates := make(map[sessionIdentity]agent.State, len(model.sessions))
	for _, session := range model.sessions {
		previousStates[sessionIdentity{provider: session.Provider, id: session.ID}] = session.State
	}

	unread := make(map[sessionIdentity]bool)
	finished := make(map[sessionIdentity]time.Time)
	for _, session := range sessions {
		identity := sessionIdentity{provider: session.Provider, id: session.ID}
		finishedAt, wasFinished := model.finishedSessions[identity]

		if session.Stale {
			// A failed poll tells us nothing about completed work; keep existing unread marks.
			if model.unreadSessions[identity] {
				unread[identity] = true
			}
			if wasFinished && now.Sub(finishedAt) < finishedHintDuration {
				finished[identity] = finishedAt
			}
			continue
		}
		if session.State != agent.StateRunning &&
			(model.unreadSessions[identity] || previousStates[identity] == agent.StateRunning) {
			unread[identity] = true
		}

		// Only an observed run becoming idle means work finished during this visit.
		if session.State == agent.StateIdle {
			if previousStates[identity] == agent.StateRunning {
				finished[identity] = now
			} else if wasFinished && now.Sub(finishedAt) < finishedHintDuration {
				finished[identity] = finishedAt
			}
		}
	}
	model.unreadSessions = unread
	model.finishedSessions = finished
}

func (model Model) finishedThisVisit(session agent.Session, now time.Time) bool {
	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	finishedAt, ok := model.finishedSessions[identity]
	return ok && session.State == agent.StateIdle && !session.Stale &&
		now.Sub(finishedAt) < finishedHintDuration
}

func (model *Model) markSelectedSessionRead() {
	if session, ok := model.selectedSession(); ok {
		identity := sessionIdentity{provider: session.Provider, id: session.ID}
		delete(model.unreadSessions, identity)
		delete(model.finishedSessions, identity)
	}
}
