package ui

import (
	"fmt"
	"strings"
	"testing"

	"a-gent/internal/agent"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestGraphUnevenProjectsFitAndRemainReachable(t *testing.T) {
	for _, counts := range [][]int{{1}, {1, 2, 3, 7, 1, 25, 2}, {40, 1, 1, 1, 1, 1, 1, 1, 1}} {
		for _, width := range []int{30, 44, 73, 88, 108, 159} {
			for _, height := range []int{14, 20, 38} {
				t.Run(fmt.Sprintf("%v/%dx%d", counts, width, height), func(t *testing.T) {
					model := unevenGraphTestModel(counts)
					layout := model.graphProjectMapLayout(width, height)
					if len(layout.pages) == 0 {
						t.Fatal("project map did not fit any cards")
					}
					seen := make(map[string]bool)
					for pageIndex, page := range layout.pages {
						root := graphNode{x: layout.rootX, y: layout.rootY, width: graphCenterWidth, height: graphRootHeight}
						cards := []graphNode{root}
						for _, branch := range page.branches {
							if seen[branch.directory] {
								t.Fatal("project appears on multiple pages")
							}
							seen[branch.directory] = true
							if layout.selectedPage(branch.representativeIndex) != pageIndex {
								t.Fatal("selection resolved to a different project page")
							}
							view := model.graphProjectView(branch, model.filteredSessions())
							if lipgloss.Width(view) != branch.width || lipgloss.Height(view) != branch.height {
								t.Fatalf("rendered card size differs from reserved size: %dx%d versus %dx%d",
									lipgloss.Width(view), lipgloss.Height(view), branch.width, branch.height)
							}
							cards = append(cards, graphNode{x: branch.x, y: branch.y, width: branch.width, height: branch.height})
						}
						assertGraphPageGeometry(t, page, root, cards, width, height)
						assertGraphTargetsReachable(t, graphPageTargets(page))
					}
					if len(seen) != len(counts) {
						t.Fatalf("kept %d of %d projects", len(seen), len(counts))
					}
				})
			}
		}
	}
}

// Mixed project sizes exercise both unequal card heights and overflowing agent pages.
func unevenGraphTestModel(counts []int) Model {
	model := graphTestModel(0)
	for project, count := range counts {
		for index := range count {
			model.sessions = append(model.sessions, agent.Session{
				ID: fmt.Sprintf("%02d-%03d", project, index), Provider: "codex",
				Name:             "作業 🛠 " + strings.Repeat("long title ", 10),
				WorkingDirectory: fmt.Sprintf("/projects/%02d/app", project), State: agent.StateRunning,
			})
		}
	}
	// A nested worker changes the first project's card height without adding a node.
	parent := model.sessions[0]
	model.sessions = append(model.sessions, agent.Session{
		ID: "worker", Provider: parent.Provider, ParentID: parent.ID,
		WorkingDirectory: parent.WorkingDirectory, State: agent.StateWaiting,
	})
	model.resizeTable()
	return model
}

func assertGraphPageGeometry(t *testing.T, page graphPage, root graphNode, cards []graphNode, width, height int) {
	t.Helper()
	occupied := make(map[graphPoint]bool)
	for _, card := range cards {
		for y := card.y; y < card.y+card.height; y++ {
			for x := card.x; x < card.x+card.width; x++ {
				point := graphPoint{x, y}
				if occupied[point] || x < 0 || x >= width || y < 0 || y >= height {
					t.Fatalf("card overlaps or leaves the viewport at %+v", point)
				}
				occupied[point] = true
			}
		}
	}
	connections := graphConnections(page, root)
	if len(connections) != len(page.branches) {
		t.Fatal("project is missing its hub connection")
	}
	for _, connection := range connections {
		for index, point := range connection.points {
			if point.x < 0 || point.x >= width || point.y < 0 || point.y >= height {
				t.Fatalf("connector leaves the viewport at %+v", point)
			}
			if index == 0 {
				continue
			}
			previous := connection.points[index-1]
			if abs(point.x-previous.x)+abs(point.y-previous.y) != 1 {
				t.Fatal("connector has a gap")
			}
			if index < len(connection.points)-1 && occupied[point] {
				t.Fatalf("connector crosses a card at %+v", point)
			}
		}
	}
}

func assertGraphTargetsReachable(t *testing.T, targets []graphNode) {
	t.Helper()
	for _, start := range targets {
		seen := map[int]bool{start.index: true}
		pending := []graphNode{start}
		for len(pending) > 0 {
			current := pending[0]
			pending = pending[1:]
			for _, direction := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				next := graphDirectionalTarget(current, targets, direction[0], direction[1])
				if seen[next] {
					continue
				}
				seen[next] = true
				for _, target := range targets {
					if target.index == next {
						pending = append(pending, target)
					}
				}
			}
		}
		if len(seen) != len(targets) {
			t.Fatalf("directional navigation reaches %d of %d cards", len(seen), len(targets))
		}
	}
}

func TestGraphSameProjectNamesKeepSeparateAgentLists(t *testing.T) {
	model := unevenGraphTestModel([]int{1, 3, 12})
	for _, group := range graphGroups(model.filteredSessions()) {
		model.table.SetCursor(group.indexes[0])
		model.openGraphProject()
		if len(model.filteredSessions()) != len(group.indexes) {
			t.Fatal("opening a same-named project changed its agent count")
		}
		for _, session := range model.filteredSessions() {
			if session.WorkingDirectory != group.directory || session.ParentID != "" {
				t.Fatal("opening a same-named project included another project's agent or a worker")
			}
		}
		if view := ansi.Strip(model.graphView()); strings.Contains(view, "worker") {
			t.Fatal("a worker became an individual agent node")
		}
		model.closeGraphProject()
		selected, ok := model.selectedSession()
		if !ok || selected.WorkingDirectory != group.directory {
			t.Fatal("returning to projects changed the selected directory")
		}
	}
}

func TestGraphProjectWorkerCountsUseProviderAndDirectory(t *testing.T) {
	model := graphTestModel(0)
	model.sessions = []agent.Session{
		{ID: "shared", Provider: "codex", WorkingDirectory: "/one/app", State: agent.StateIdle},
		{ID: "shared", Provider: "claude", WorkingDirectory: "/two/app", State: agent.StateIdle},
		{ID: "worker", Provider: "codex", ParentID: "shared", WorkingDirectory: "/one/app", State: agent.StateRunning},
		{ID: "worker", Provider: "claude", ParentID: "shared", WorkingDirectory: "/two/app", State: agent.StateWaiting},
		{ID: "nested", Provider: "claude", ParentID: "worker", WorkingDirectory: "/two/app", State: agent.StateIdle},
	}
	model.resizeTable()
	parents := model.filteredSessions()
	codex := model.graphProjectSummary("/one/app", parents)
	claude := model.graphProjectSummary("/two/app", parents)
	if len(codex.sessions) != 1 || codex.workers != 1 || codex.runningWorkers != 1 || codex.waitingWorkers != 0 {
		t.Fatalf("Codex project inherited another provider's workers: %+v", codex)
	}
	if len(claude.sessions) != 1 || claude.workers != 2 || claude.runningWorkers != 0 || claude.waitingWorkers != 1 {
		t.Fatalf("Claude project lost nested workers or inherited another provider's status: %+v", claude)
	}
}
