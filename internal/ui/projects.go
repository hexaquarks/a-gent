package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maximumProjectRows = 6

// WithProjectPins loads the user's pinned projects from the supplied file.
func WithProjectPins(path string) ModelOption {
	return func(model *Model) {
		model.projectPinsPath = path
		model.pinnedProjects = make(map[string]bool)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return
		}
		if err == nil {
			err = json.Unmarshal(data, &model.pinnedProjects)
		}
		if err != nil {
			model.notice = "Could not load project pins"
		}
	}
}

func (model Model) saveProjectPins() error {
	if model.projectPinsPath == "" {
		return nil
	}
	data, err := json.Marshal(model.pinnedProjects)
	if err != nil {
		return err
	}
	directory := filepath.Dir(model.projectPinsPath)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, "project-pins-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), model.projectPinsPath)
}

func (model Model) matchingProjects() []string {
	projects := model.projects()
	if !model.projectSearching || model.projectQuery == "" {
		return projects
	}
	matches := make([]string, 0, len(projects))
	for _, project := range projects {
		if strings.Contains(strings.ToLower(project), strings.ToLower(model.projectQuery)) {
			matches = append(matches, project)
		}
	}
	return matches
}

func (model Model) projectItemStart() int {
	count := len(sidebarViews())
	providers := model.providers()
	if len(providers) > 3 {
		count++
	}
	if len(providers) <= 3 || model.agentsExpanded {
		count += len(providers)
	}
	return count
}

func (model Model) visibleProjectRange() (int, int) {
	count := len(model.matchingProjects())
	visibleRows := model.projectRowCapacity()
	start := 0
	cursor := model.sidebarCursor - model.projectItemStart()
	if model.sidebarFocus && cursor >= visibleRows {
		start = cursor - visibleRows + 1
	}
	start = min(start, max(0, count-visibleRows))
	return start, min(start+visibleRows, count)
}

// Reserve room for views, three agent types, and section spacing above projects.
func (model Model) projectRowCapacity() int {
	return max(1, min(maximumProjectRows, model.sidebarHeight()-model.projectTop()-3))
}

func (model Model) projectItemView(item sidebarItem, focused, selected bool) string {
	star := " "
	if model.pinnedProjects[item.project] {
		star = "★"
	}
	markerStyle := model.projectStyle(item.project)
	style := mainTextStyle
	countStyle := mutedStyle
	if selected {
		style = style.Bold(true)
	}
	if focused {
		style = style.Background(lipgloss.Color(colorSelection))
		markerStyle = markerStyle.Background(lipgloss.Color(colorSelection))
		countStyle = countStyle.Background(lipgloss.Color(colorSelection))
	}
	return model.renderSidebarRow(item.label, "\uf07b", star, fmt.Sprint(model.projectCount(item.project)), style, markerStyle, countStyle)
}

func (model Model) updateProjectSearch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		return model, tea.Quit
	case tea.KeyEsc:
		model.projectSearching = false
		model.projectQuery = ""
		model.sidebarCursor = min(model.sidebarCursor, len(model.sidebarItems())-1)
	case tea.KeyEnter:
		projects := model.matchingProjects()
		index := model.sidebarCursor - model.projectItemStart()
		if index >= 0 && index < len(projects) {
			model.selectedProject = projects[index]
			model.selectedProvider = ""
			model.projectSearching = false
			model.projectQuery = ""
			for index, item := range model.sidebarItems() {
				if item.project == model.selectedProject && item.view == "" && item.provider == "" && !item.allTypes {
					model.sidebarCursor = index
					break
				}
			}
			model.updateTableRows()
		}
	case tea.KeyUp, tea.KeyDown:
		count := len(model.matchingProjects())
		if count > 0 {
			offset := 1
			if key.Type == tea.KeyUp {
				offset = -1
			}
			start := model.projectItemStart()
			model.sidebarCursor = start + (model.sidebarCursor-start+offset+count)%count
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		query := []rune(model.projectQuery)
		if len(query) > 0 {
			model.projectQuery = string(query[:len(query)-1])
		}
		model.sidebarCursor = model.projectItemStart()
	case tea.KeyRunes:
		model.projectQuery += safeDisplayText(string(key.Runes))
		model.sidebarCursor = model.projectItemStart()
	case tea.KeySpace:
		model.projectQuery += " "
		model.sidebarCursor = model.projectItemStart()
	}
	return model, nil
}

func (model Model) projectTop() int {
	return max(9, min(11, model.sidebarHeight()-4))
}
