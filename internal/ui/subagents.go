package ui

import (
	"strings"

	"a-gent/internal/agent"
)

// Sorting applies to siblings; children remain directly beneath their parent.
// A child whose parent is filtered out still appears with its subagent marker.
func groupSubagents(sessions []agent.Session) []agent.Session {
	hasChildren := false
	for _, session := range sessions {
		hasChildren = hasChildren || session.ParentID != ""
	}
	if !hasChildren {
		return sessions
	}

	present := make(map[sessionIdentity]bool, len(sessions))
	children := make(map[sessionIdentity][]agent.Session)
	for _, session := range sessions {
		present[sessionIdentity{session.Provider, session.ID}] = true
		parent := sessionIdentity{session.Provider, session.ParentID}
		children[parent] = append(children[parent], session)
	}

	ordered := make([]agent.Session, 0, len(sessions))
	visited := make(map[sessionIdentity]bool, len(sessions))
	var appendSession func(agent.Session)
	appendSession = func(session agent.Session) {
		identity := sessionIdentity{session.Provider, session.ID}
		if visited[identity] {
			return
		}
		visited[identity] = true
		ordered = append(ordered, session)
		for _, child := range children[identity] {
			appendSession(child)
		}
	}
	for _, session := range sessions {
		if session.ParentID == "" || !present[sessionIdentity{session.Provider, session.ParentID}] {
			appendSession(session)
		}
	}
	// Malformed ancestry must not hide sessions or recurse forever.
	for _, session := range sessions {
		appendSession(session)
	}
	return ordered
}

func (model Model) listSessionName(session agent.Session) string {
	if session.ParentID == "" {
		return sessionDisplayName(session)
	}
	parents := make(map[sessionIdentity]string, len(model.sessions))
	for _, candidate := range model.sessions {
		parents[sessionIdentity{candidate.Provider, candidate.ID}] = candidate.ParentID
	}
	depth := 1
	seen := map[string]bool{session.ID: true}
	for parent := session.ParentID; parent != "" && !seen[parent]; {
		seen[parent] = true
		parent = parents[sessionIdentity{session.Provider, parent}]
		if parent != "" {
			depth++
		}
	}
	// Bound indentation so deeply nested agents retain a readable title.
	return strings.Repeat("  ", min(depth, 4)) + "↳ " + sessionDisplayName(session)
}

func (model Model) sessionWorkspace(session agent.Session) agent.Session {
	seen := make(map[sessionIdentity]bool)
	for session.ParentID != "" {
		identity := sessionIdentity{session.Provider, session.ID}
		if seen[identity] {
			break
		}
		seen[identity] = true
		found := false
		for _, parent := range model.sessions {
			if parent.Provider == session.Provider && parent.ID == session.ParentID {
				session = parent
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return session
}
