package ui

import (
	"slices"
	"strings"

	"a-gent/internal/agent"
)

const maxSessionTreeDepth = 4

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
	name := session.Name
	if strings.TrimSpace(safeDisplayText(name)) == "" {
		name = "Untitled session"
	}
	if session.ParentID == "" {
		return name
	}

	visible := model.filteredSessions()
	byIdentity := make(map[sessionIdentity]agent.Session, len(visible))
	lastSibling := make(map[sessionIdentity]sessionIdentity)
	for _, candidate := range visible {
		identity := sessionIdentity{candidate.Provider, candidate.ID}
		byIdentity[identity] = candidate
		lastSibling[sessionIdentity{candidate.Provider, candidate.ParentID}] = identity
	}

	// Draw only visible ancestry, and bound malformed or deeply nested trees
	// so their branches cannot consume the entire task column.
	path := []agent.Session{session}
	seen := map[string]bool{session.ID: true}
	for len(path) < maxSessionTreeDepth {
		parent, ok := byIdentity[sessionIdentity{session.Provider, session.ParentID}]
		if !ok || parent.ParentID == "" || seen[parent.ID] {
			break
		}
		path = append(path, parent)
		seen[parent.ID] = true
		session = parent
	}
	slices.Reverse(path)

	var gutter strings.Builder
	for index, node := range path {
		identity := sessionIdentity{node.Provider, node.ID}
		last, known := lastSibling[sessionIdentity{node.Provider, node.ParentID}]
		isLast := !known || last == identity
		if index == len(path)-1 {
			if isLast {
				gutter.WriteString("└─ ")
			} else {
				gutter.WriteString("├─ ")
			}
		} else if isLast {
			gutter.WriteString("   ")
		} else {
			gutter.WriteString("│  ")
		}
	}
	return gutter.String() + name
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
