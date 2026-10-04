package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"a-gent/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var sortColumns = []string{"Last active", "Session", "Agent", "Status", "Directory"}

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

func (sort sessionSort) directionLabel() string {
	if sort.column == "Last active" {
		if sort.descending {
			return "newest first"
		}
		return "oldest first"
	}
	if sort.descending {
		return "Z–A"
	}
	return "A–Z"
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
		return displayState(session.State)
	case "Directory":
		if session.WorkingDirectory == "" {
			return ""
		}
		return filepath.Base(session.WorkingDirectory)
	default:
		return ""
	}
}

func (model *Model) openSortMenu() {
	model.sortMenuOpen = true
	model.sortMenuDraft = model.sort
	model.sortMenuCursor = max(0, slices.Index(sortColumns, model.sort.column))
}

func (model Model) updateSortMenu(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c":
		return model, tea.Quit
	case "esc", "s":
		model.sortMenuOpen = false
	case "j", "down", "k", "up":
		offset := 1
		if message.String() == "k" || message.String() == "up" {
			offset = -1
		}
		model.sortMenuCursor = (model.sortMenuCursor + offset + len(sortColumns)) % len(sortColumns)
		column := sortColumns[model.sortMenuCursor]
		model.sortMenuDraft = sessionSort{column: column, descending: column == "Last active"}
		if column == model.sort.column {
			model.sortMenuDraft = model.sort
		}
	case "left", "h":
		model.sortMenuDraft.descending = false
	case "right", "l":
		model.sortMenuDraft.descending = true
	case "enter":
		model.sort = model.sortMenuDraft
		model.sortMenuOpen = false
		model.updateTableRows()
	}
	return model, nil
}

func (model Model) sessionHeadingView() string {
	width := model.table.Width() - panelStyle.GetHorizontalFrameSize()
	indicator := mutedStyle.Render("Sort: ") + accentStyle.Render(model.sort.column+" "+model.sort.arrow())
	titleWidth := max(0, width-lipgloss.Width(indicator)-2)
	title := model.panelTitleStyle().Render(runewidth.Truncate(model.sessionTitle(), titleWidth, "…"))
	gap := strings.Repeat(" ", max(0, width-lipgloss.Width(title)-lipgloss.Width(indicator)))
	return title + gap + indicator
}

func (model Model) sortMenuView() string {
	width := model.table.Width() - panelStyle.GetHorizontalFrameSize()
	height := model.table.Height() + 2
	lines := []string{sectionStyle.Render("SORT BY"), ""}
	visibleOptions := min(len(sortColumns), max(1, height-len(lines)))
	start := max(0, model.sortMenuCursor-visibleOptions+1)
	for index := start; index < start+visibleOptions; index++ {
		label := "  " + sortColumns[index]
		style := mainTextStyle
		if index == model.sortMenuCursor {
			label = "› " + sortColumns[index] + " " + model.sortMenuDraft.arrow() + "  " + model.sortMenuDraft.directionLabel()
			style = accentStyle.Background(lipgloss.Color(colorSelection))
		}
		lines = append(lines, style.Width(width).Render(runewidth.Truncate(label, width, "…")))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}
