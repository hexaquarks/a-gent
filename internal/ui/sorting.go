package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type sessionSort struct {
	column     string
	descending bool
}

type sessionIdentity struct {
	provider string
	id       string
}

func (sort sessionSort) arrow() string {
	if sort.descending {
		return "↓"
	}
	return "↑"
}

func (model Model) sortSessions(sessions []agent.Session) {
	slices.SortStableFunc(sessions, func(left, right agent.Session) int {
		comparison := 0
		if model.sort.column == "Last active" {
			// Unknown activity stays last in both directions.
			if left.LastActiveAt.IsZero() != right.LastActiveAt.IsZero() {
				if left.LastActiveAt.IsZero() {
					return 1
				}
				return -1
			}
			comparison = left.LastActiveAt.Compare(right.LastActiveAt)
		} else {
			leftValue := sessionSortValue(left, model.sort.column)
			rightValue := sessionSortValue(right, model.sort.column)
			if (leftValue == "") != (rightValue == "") {
				if leftValue == "" {
					return 1
				}
				return -1
			}
			comparison = strings.Compare(strings.ToLower(leftValue), strings.ToLower(rightValue))
		}
		if comparison != 0 {
			if model.sort.descending {
				return -comparison
			}
			return comparison
		}

		// Provider output order can change between polls. Identity breaks ties
		// consistently so equal values do not shuffle on every refresh.
		if comparison = cmp.Compare(left.Provider, right.Provider); comparison != 0 {
			return comparison
		}
		return cmp.Compare(left.ID, right.ID)
	})
}

func sessionSortValue(session agent.Session, column string) string {
	switch column {
	case "Session":
		return strings.TrimSpace(session.Name)
	case "Agent":
		return session.Provider
	case "Status":
		return displayState(sessionState(session))
	case "Directory":
		if session.WorkingDirectory == "" {
			return ""
		}
		return filepath.Base(session.WorkingDirectory)
	default:
		return ""
	}
}

// Cycle through the columns in their displayed order, wrapping at the end.
func (model *Model) cycleSortColumn() {
	columns := model.table.Columns()
	if len(columns) == 0 {
		return
	}

	nextIndex := 0
	for index, column := range columns {
		if column.Title == model.sort.column {
			nextIndex = (index + 1) % len(columns)
			break
		}
	}
	column := columns[nextIndex].Title
	model.sort = sessionSort{column: column, descending: column == "Last active"}
	model.updateTableRows()
}

func (model Model) sessionHeadingView() string {
	width := model.table.Width() - panelStyle.GetHorizontalFrameSize()
	indicator := mutedStyle.Render("Sort: ") + accentStyle.Render(model.sort.column+" "+model.sort.arrow())
	titleWidth := max(0, width-lipgloss.Width(indicator)-2)
	title := model.panelTitleStyle().Render(runewidth.Truncate(model.sessionTitle(), titleWidth, "…"))
	gap := strings.Repeat(" ", max(0, width-lipgloss.Width(title)-lipgloss.Width(indicator)))
	return title + gap + indicator
}
