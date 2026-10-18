package ui

import (
	"strings"
	"testing"

	"a-gent/internal/agent"
	"github.com/charmbracelet/x/ansi"
)

func graphProjectLayoutForTest(count, projects, width, height int) graphLayout {
	model := graphTestModel(count)
	model.sessions = graphTestSessions(count, projects)
	return model.graphProjectMapLayout(width, height)
}

func TestGraphProjectMapShowsParentRowsAndNestedWorkerTotals(t *testing.T) {
	model := graphTestModel(2)
	model.width, model.height = 190, 50
	parent := model.sessions[0]
	model.sessions = append(model.sessions,
		agent.Session{ID: "worker", Provider: parent.Provider, ParentID: parent.ID,
			Name: "HIDDEN_WORKER", WorkingDirectory: parent.WorkingDirectory, State: agent.StateIdle},
		agent.Session{ID: "nested", Provider: parent.Provider, ParentID: "worker",
			Name: "HIDDEN_NESTED", WorkingDirectory: parent.WorkingDirectory, State: agent.StateRunning},
	)
	model.resizeTable()
	layout := model.graphBrowserLayout()
	for _, branch := range layout.pages[0].branches {
		if branch.directory != parent.WorkingDirectory {
			continue
		}
		view := ansi.Strip(model.graphProjectView(branch, model.filteredSessions()))
		for _, text := range []string{"3 agents", parent.Name, "2 workers", "1 running"} {
			if !strings.Contains(view, text) {
				t.Fatalf("project card omitted %q:\n%s", text, view)
			}
		}
		if strings.Contains(view, "HIDDEN_") || len(branch.nodes) != 0 {
			t.Fatal("workers became separate graph nodes or rows")
		}
		return
	}
	t.Fatal("parent project is missing")
}

func TestGraphProjectMapCardsStayWithinBoundsAndKeepEveryProject(t *testing.T) {
	for _, size := range [][2]int{{40, 18}, {90, 20}, {159, 42}} {
		model := graphTestModel(100)
		model.sessions = graphTestSessions(100, 17)
		layout := model.graphProjectMapLayout(size[0], size[1])
		seen := make(map[string]bool)
		for _, page := range layout.pages {
			root := graphNode{x: layout.rootX, y: layout.rootY, width: graphCenterWidth, height: graphRootHeight}
			occupied := make(map[graphPoint]bool)
			cards := []graphNode{root}
			for _, branch := range page.branches {
				if seen[branch.directory] {
					t.Fatal("project repeated on another page")
				}
				seen[branch.directory] = true
				cards = append(cards, graphNode{x: branch.x, y: branch.y, width: branch.width, height: branch.height})
				view := model.graphProjectView(branch, model.filteredSessions())
				if len(strings.Split(view, "\n")) != branch.height {
					t.Fatal("project content exceeds its reserved card height")
				}
				if !strings.Contains(ansi.Strip(view), "more sessions") {
					t.Fatal("large project hid rows without a more count")
				}
			}
			for _, card := range cards {
				for y := card.y; y < card.y+card.height; y++ {
					for x := card.x; x < card.x+card.width; x++ {
						point := graphPoint{x, y}
						if occupied[point] || x < 0 || x >= size[0] || y < 0 || y >= size[1] {
							t.Fatalf("overlapping or clipped project card at %+v in %v", point, size)
						}
						occupied[point] = true
					}
				}
			}
		}
		if len(seen) != 17 {
			t.Fatalf("kept %d of 17 projects at %v", len(seen), size)
		}
	}
}

func TestGraphOverviewContainsOnlyProjectCards(t *testing.T) {
	layout := graphProjectLayoutForTest(100, 5, 130, 38)
	if len(layout.pages) != 1 || abs(layout.rootX+graphCenterWidth/2-65) > 1 {
		t.Fatal("overview should fit five projects around a centered hub")
	}
	seen := make(map[string]bool)
	for _, branch := range layout.pages[0].branches {
		if len(branch.nodes) != 0 || branch.height == 0 {
			t.Fatal("overview must contain project cards only")
		}
		seen[branch.directory] = true
	}
	if len(seen) != 5 {
		t.Fatal("overview lost or duplicated a project")
	}
}
