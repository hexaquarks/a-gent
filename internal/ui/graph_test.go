package ui

import (
	"strings"
	"testing"
	"time"

	"a-gent/internal/agent"
	"a-gent/internal/polling"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestGraphLayoutCyclePreservesSelection(t *testing.T) {
	model := graphTestModel(9)
	selected, _ := model.selectedSession()
	for _, layout := range []string{"list", "board", "graph"} {
		model = sendProjectKey(model, runeKey('m'))
		current, _ := model.selectedSession()
		if current.ID != selected.ID || model.boardVisible != (layout == "board") ||
			model.graphVisible != (layout == "graph") {
			t.Fatalf("layout %s changed selection or mode", layout)
		}
	}
}

func TestGraphBoundsAndSelectedVisibility(t *testing.T) {
	for _, size := range [][2]int{{190, 50}, {170, 35}, {160, 50}, {120, 29}, {80, 24}, {40, 15}, {24, 8}, {10, 5}} {
		for _, count := range []int{0, 1, 9, 100} {
			model := graphTestModel(count)
			model.width, model.height = size[0], size[1]
			model.resizeTable()
			if count > 0 {
				model.table.SetCursor(count - 1)
			}
			view := model.View()
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%dx%d count %d overflowed: %dx%d", size[0], size[1], count, lipgloss.Width(view), lipgloss.Height(view))
			}
			if count > 0 && size[1] >= 15 {
				selected, _ := model.selectedSession()
				if !strings.Contains(ansi.Strip(model.graphView()), projectName(selected.WorkingDirectory)) {
					t.Fatalf("selected project missing at %v count %d", size, count)
				}
			}
		}
	}
}

func TestGraphPageNavigation(t *testing.T) {
	model := graphTestModel(100)
	model.width = 40
	model.resizeTable()
	model.table.SetCursor(graphGroups(model.filteredSessions())[0].indexes[0])
	layout := model.graphBrowserLayout()
	model = sendProjectKey(model, runeKey(']'))
	if layout.selectedPage(model.table.Cursor()) != 1 {
		t.Fatal("page shortcut did not advance")
	}
	model = sendProjectKey(model, runeKey('['))
	if layout.selectedPage(model.table.Cursor()) != 0 {
		t.Fatal("page shortcut did not return")
	}
}

func TestGraphPreservesIdentityAcrossPollsAndUsesOnlySelectedPreview(t *testing.T) {
	model := graphTestModel(9)
	model.width, model.height = 190, 50
	model.resizeTable()
	model.openGraphProject()
	selected, _ := model.selectedSession()
	model.previewCache = make(map[sessionIdentity]previewEntry)
	for _, session := range model.sessions {
		model.previewCache[sessionIdentity{session.Provider, session.ID}] = previewEntry{
			loaded: true, checked: time.Now(), activity: &agent.Activity{Text: "OUTPUT_" + session.ID, At: time.Now()},
		}
	}
	updated := append([]agent.Session(nil), model.sessions...)
	for index := range updated {
		updated[index].LastActiveAt = time.Now().Add(time.Duration(index) * time.Minute)
	}
	next, _ := model.Update(polling.Update{Provider: "codex", Sessions: updated})
	model = next.(Model)
	current, _ := model.selectedSession()
	if current.ID != selected.ID {
		t.Fatal("poll moved selection")
	}
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "OUTPUT_"+selected.ID) || strings.Count(view, "OUTPUT_") != 1 {
		t.Fatalf("incorrect preview:\n%s", view)
	}
}

func TestGraphShowsParentsAndRespectsFilters(t *testing.T) {
	model := graphTestModel(2)
	model.sessions[1].ParentID = model.sessions[0].ID
	model.sessions[1].WorkingDirectory = model.sessions[0].WorkingDirectory
	model.selectedProject = model.sessions[0].WorkingDirectory
	model.updateTableRows()
	if len(model.filteredSessions()) != 1 || strings.Contains(ansi.Strip(model.graphView()), "subagent") {
		t.Fatal("graph must show only the parent node")
	}
	model.selectedProvider = "claude"
	model.updateTableRows()
	if !strings.Contains(ansi.Strip(model.graphView()), "No sessions match") {
		t.Fatal("provider filter ignored")
	}
}

func TestGraphMovementRespectsSidebarAndHelp(t *testing.T) {
	for _, owner := range []string{"sidebar", "help", "search", "preview"} {
		model := graphTestModel(9)
		selected, _ := model.selectedSession()
		switch owner {
		case "sidebar":
			model.sidebarFocus = true
		case "help":
			model.helpOpen = true
		case "search":
			model.projectSearching = true
		case "preview":
			model.previewExpanded = true
		}
		for _, key := range []tea.KeyMsg{runeKey('l'), runeKey(']')} {
			model = sendProjectKey(model, key)
		}
		if current, _ := model.selectedSession(); current.ID != selected.ID {
			t.Fatalf("%s lost key ownership", owner)
		}
	}
}

func TestGraphColoredUnicodeRenderingStaysBounded(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	model := graphTestModel(9)
	model.width, model.height = 160, 48
	model.sessions[0].Name = "Review 界面 👩‍💻 é\x1b[2J malicious title"
	model.sessions[0].WorkingDirectory = "/projects/界面"
	model.resizeTable()

	view := model.graphView()
	width, height := model.graphDimensions()
	const maximumStyledBytesPerCell = 64
	if len(view) > width*height*maximumStyledBytesPerCell {
		t.Fatalf("graph duplicated ANSI sequences: %d bytes", len(view))
	}
	if strings.Contains(view, "\x1b[2J") {
		t.Fatal("session title injected a terminal control sequence")
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) != width {
			t.Fatalf("wide characters broke graph width: %d, want %d", ansi.StringWidth(line), width)
		}
	}
}

func TestGraphNavigationVisitsProjectsAndTheirAgents(t *testing.T) {
	model := graphTestModel(9)
	model.table.SetCursor(model.graphSelectedIndex())
	seen := make(map[string]bool)
	for _, key := range []rune{'j', 'j', 'l', 'k'} {
		session, _ := model.selectedSession()
		seen[session.WorkingDirectory] = true
		model = sendProjectKey(model, runeKey(key))
	}
	session, _ := model.selectedSession()
	seen[session.WorkingDirectory] = true
	if len(seen) != 5 {
		t.Fatal("spatial navigation must reach all project cards")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if !model.graphProjectOpen {
		t.Fatal("Enter did not open the project")
	}
	directory := model.graphProjectDirectory
	for _, session := range model.filteredSessions() {
		if session.WorkingDirectory != directory || session.ParentID != "" {
			t.Fatal("cluster includes an unrelated agent or a subagent")
		}
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	session, _ = model.selectedSession()
	if model.graphProjectOpen || session.WorkingDirectory != directory {
		t.Fatal("back did not retain the selected project")
	}
}

func TestGraphCompactUnreadMarkerSurvivesLongTitles(t *testing.T) {
	model := graphTestModel(1)
	session := model.sessions[0]
	session.Name = strings.Repeat("long title ", 20)
	model.unreadSessions = map[sessionIdentity]bool{{session.Provider, session.ID}: true}

	for _, height := range []int{graphTinyNodeRows, graphCompactNodeRows} {
		view := ansi.Strip(model.graphNodeView(session, 0, 24, height))
		if !strings.Contains(view, "•") || strings.Contains(view, "›") {
			t.Fatalf("compact node hid unread marker or retained a selection label: %s", view)
		}
		heading := strings.Split(view, "\n")[1]
		if !strings.HasPrefix(heading, "│ "+session.Provider) || !strings.HasSuffix(heading, " │") {
			t.Fatalf("agent heading lacks padding or retains a numeric label: %s", heading)
		}
	}
}

func TestGraphNumericSelection(t *testing.T) {
	model := graphTestModel(9)
	model.openGraphProject()
	model = sendProjectKey(model, runeKey('2'))
	if model.graphSelectedIndex() != graphPageTargets(model.graphBrowserLayout().pages[0])[1].index {
		t.Fatal("number did not select its agent")
	}
}

func TestGraphProjectShowsItsTotalAgentCount(t *testing.T) {
	model := graphTestModel(2)
	model.sessions[1].WorkingDirectory = model.sessions[0].WorkingDirectory
	branch := graphBranch{
		directory: model.sessions[0].WorkingDirectory, width: graphProjectMapMinimumWidth, height: 6,
		nodes: []graphNode{{index: 0}},
	}
	view := ansi.Strip(model.graphProjectView(branch, model.sessions))
	if !strings.Contains(view, "2 agents") {
		t.Fatalf("project count is misleading: %s", view)
	}
}

func TestGraphChildAttentionUsesParentNode(t *testing.T) {
	model := graphTestModel(2)
	parent := model.sessions[0]
	model.sessions[1].ParentID = parent.ID
	model.sessions[1].Provider = parent.Provider
	model.sessions[1].State = agent.StateWaiting
	model.sessions[0].State = agent.StateIdle
	child := model.sessions[1]
	model.unreadSessions = map[sessionIdentity]bool{
		{provider: child.Provider, id: child.ID}: true,
	}

	for _, view := range []sidebarView{needsYouView, unseenView} {
		model.selectedView = view
		model.updateTableRows()
		sessions := model.filteredSessions()
		if len(sessions) != 1 || sessions[0].ID != parent.ID {
			t.Fatalf("%s should show the owning parent: %+v", view, sessions)
		}
	}
	if !strings.Contains(ansi.Strip(model.graphNodeView(parent, 0, 18, 6)), "•") {
		t.Fatal("parent node lost child unread indicator")
	}
	model.markSelectedSessionRead()
	if len(model.unreadSessions) != 0 {
		t.Fatal("opening the parent did not acknowledge its children")
	}
}

func TestGraphOmitsBottomPreview(t *testing.T) {
	for _, size := range [][2]int{{120, 29}, {70, 24}, {190, 50}} {
		model := graphTestModel(2)
		model.width, model.height = size[0], size[1]
		model.resizeTable()
		view := ansi.Strip(model.View())
		if strings.Contains(view, "FOCUS") || strings.Contains(view, "v details / diff") {
			t.Fatal("Graph retained the bottom selected-agent preview")
		}
		_, height := model.graphDimensions()
		if height != model.bodyHeight()-model.boardHeadingHeight() {
			t.Fatal("Graph did not reclaim the preview height")
		}
	}
}

func TestGraphClickOpensProjectAndHubReturns(t *testing.T) {
	for _, size := range [][2]int{{190, 50}, {120, 29}, {40, 24}} {
		model := graphTestModel(9)
		model.width, model.height = size[0], size[1]
		model.resizeTable()
		layout := model.graphBrowserLayout()
		page := layout.pages[layout.selectedPage(model.graphSelectedIndex())]
		target := graphPageTargets(page)[0]
		originX, originY := model.graphOrigin()
		click := tea.MouseMsg{
			X: originX + target.x + 1, Y: originY + target.y + 1,
			Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
		}
		next, _ := model.Update(click)
		model = next.(Model)
		if !model.graphProjectOpen {
			t.Fatalf("project click did not enter at %v", size)
		}
		layout = model.graphBrowserLayout()
		click.X, click.Y = originX+layout.rootX+1, originY+layout.rootY+1
		next, _ = model.Update(click)
		model = next.(Model)
		if model.graphProjectOpen {
			t.Fatalf("hub click did not return at %v", size)
		}
	}
}

func TestGraphOverviewDoesNotReadAgentPreviewOrAcknowledgeUnread(t *testing.T) {
	model := graphTestModel(9)
	source := &previewSource{}
	model.previewSources = map[string]agent.PreviewSource{"codex": source}
	session, _ := model.selectedSession()
	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	model.unreadSessions = map[sessionIdentity]bool{identity: true}
	if command := model.syncPreview(); command != nil {
		t.Fatal("project overview tried to read an arbitrary agent preview")
	}
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	if !model.graphProjectOpen || !model.unreadSessions[identity] {
		t.Fatal("opening a project acknowledged an agent")
	}
	if model.previewKey != identity || model.previewCancel == nil {
		t.Fatal("cluster did not load its selected agent")
	}
}

func TestGraphClusterKeepsFiltersAndSurvivesEmptyUpdates(t *testing.T) {
	model := graphTestModel(9)
	model.selectedView = activeView
	model.updateTableRows()
	model.openGraphProject()
	directory := model.graphProjectDirectory
	filtered := model.filteredSessions()
	filtered[0].State = agent.StateIdle
	for index := range model.sessions {
		if model.sessions[index].ID == filtered[0].ID {
			model.sessions[index].State = agent.StateIdle
		}
	}
	for _, session := range model.filteredSessions() {
		if session.State != agent.StateRunning || session.WorkingDirectory != directory {
			t.Fatal("opening a cluster bypassed the active filter")
		}
	}
	next, _ := model.Update(polling.Update{Provider: "codex", Sessions: nil})
	model = next.(Model)
	if len(model.filteredSessions()) != 0 {
		t.Fatal("departed sessions remain in the cluster")
	}
	model.View()
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	if model.graphProjectOpen {
		t.Fatal("empty cluster trapped navigation")
	}
}

func TestGraphClusterNavigationReachesEveryAgentAcrossPages(t *testing.T) {
	model := graphTestModel(40)
	for index := range model.sessions {
		model.sessions[index].WorkingDirectory = "/large-project"
	}
	model.updateTableRows()
	model.openGraphProject()
	model.table.SetCursor(model.graphBrowserLayout().pages[0].branches[0].nodes[0].index)
	seen := make(map[string]bool)
	layout := model.graphBrowserLayout()
	record := func() {
		session, _ := model.selectedSession()
		seen[session.ID] = true
	}
	for _, page := range layout.pages {
		targets := graphPageTargets(page)
		record()
		for range (len(targets)+1)/2 - 1 {
			model = sendProjectKey(model, runeKey('j'))
			record()
		}
		if len(targets) > 1 {
			model = sendProjectKey(model, runeKey('l'))
			record()
			for range len(targets)/2 - 1 {
				model = sendProjectKey(model, runeKey('k'))
				record()
			}
		}
		model = sendProjectKey(model, runeKey(']'))
	}
	if len(seen) != 40 {
		t.Fatalf("visited %d of 40 agents", len(seen))
	}
	selected, _ := model.selectedSession()
	model = sendProjectKey(model, runeKey('m'))
	current, _ := model.selectedSession()
	if current.ID != selected.ID || model.graphProjectOpen {
		t.Fatal("changing layout lost the selected agent or retained cluster scope")
	}
}

func TestGraphHorizontalNavigationAndAgentClick(t *testing.T) {
	model := graphTestModel(9)
	page := model.graphBrowserLayout().pages[0]
	targets := graphPageTargets(page)
	model.table.SetCursor(targets[0].index)
	model = sendProjectKey(model, tea.KeyMsg{Type: tea.KeyRight})
	selected := model.graphSelectedIndex()
	if selected == targets[0].index {
		t.Fatal("right arrow did not select a project on the right")
	}
	model = sendProjectKey(model, runeKey('h'))
	if model.graphSelectedIndex() == selected {
		t.Fatal("h did not select a project on the left")
	}

	model.openGraphProject()
	layout := model.graphBrowserLayout()
	targets = graphPageTargets(layout.pages[0])
	if len(targets) < 2 {
		t.Fatal("fixture needs two parent agents")
	}
	target := targets[1]
	session := model.filteredSessions()[target.index]
	identity := sessionIdentity{provider: session.Provider, id: session.ID}
	model.unreadSessions = map[sessionIdentity]bool{identity: true}
	originX, originY := model.graphOrigin()
	next, _ := model.Update(tea.MouseMsg{
		X: originX + target.x + 1, Y: originY + target.y + 1,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	model = next.(Model)
	current, _ := model.selectedSession()
	if current.ID != session.ID || !model.unreadSessions[identity] || !model.graphProjectOpen {
		t.Fatal("agent click should select the agent without opening or acknowledging it")
	}
}

func TestGraphProjectNamesHaveNoNumericPrefix(t *testing.T) {
	model := graphTestModel(9)
	for _, branch := range model.graphBrowserLayout().pages[0].branches {
		view := ansi.Strip(model.graphProjectView(branch, model.filteredSessions()))
		if !strings.Contains(view, projectName(branch.directory)) {
			t.Fatal("project name missing")
		}
		for _, prefix := range []string{"1 ", "2 ", "3 ", "4 ", "5 "} {
			if strings.Contains(view, prefix+projectName(branch.directory)) {
				t.Fatalf("project name is still numbered: %s", view)
			}
		}
	}
}

func TestGraphSelectionRetainsBaseColors(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	model := graphTestModel(2)
	model.sessions[0].State = agent.StateError
	model.updateTableRows()
	sessions := model.filteredSessions()
	model.table.SetCursor(0)
	selectedAgentBorder := strings.Split(model.graphNodeView(sessions[0], 0, 28, graphFullNodeRows), "\n")[0]
	branch := model.graphBrowserLayout().pages[0].branches[0]
	selectedProject := model.graphProjectView(branch, sessions)

	model.table.SetCursor(1)
	unselectedAgentBorder := strings.Split(model.graphNodeView(sessions[0], 0, 28, graphFullNodeRows), "\n")[0]
	unselectedProject := model.graphProjectView(branch, sessions)
	if selectedAgentBorder != unselectedAgentBorder ||
		strings.Split(selectedProject, "\n")[0] != strings.Split(unselectedProject, "\n")[0] {
		t.Fatal("selection changed the underlying status or project border color")
	}
	if selectedProject == unselectedProject {
		t.Fatal("selection did not tint the project background")
	}
	view := ansi.Strip(model.graphView())
	if strings.Contains(view, " selected ") || strings.Contains(view, "╔") || strings.Contains(view, "›") {
		t.Fatal("selection retained its outer frame, label, or arrow")
	}
}

func TestGraphQuitBacktracksBeforeQuitting(t *testing.T) {
	for _, key := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
		model := graphTestModel(9)
		model.openGraphProject()
		directory := model.graphProjectDirectory
		next, command := model.Update(key)
		model = next.(Model)
		selected, _ := model.selectedSession()
		if command != nil || model.graphProjectOpen || !model.graphVisible ||
			selected.WorkingDirectory != directory {
			t.Fatalf("%s did not return to the owning project", key.String())
		}
		next, command = model.Update(runeKey('q'))
		if command == nil || command() != tea.Quit() {
			t.Fatal("q from overview should quit")
		}
	}
	model := graphTestModel(9)
	model.openGraphProject()
	model.previewExpanded = true
	next, command := model.Update(runeKey('q'))
	model = next.(Model)
	if command != nil || model.previewExpanded || !model.graphProjectOpen {
		t.Fatal("q should first close the expanded agent preview")
	}
	_, command = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if command == nil || command() != tea.Quit() {
		t.Fatal("Ctrl+C should still quit from inside a project")
	}
}
