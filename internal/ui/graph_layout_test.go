package ui

import (
	"fmt"
	"testing"

	"a-gent/internal/agent"
)

func graphTestSessions(count, projects int) []agent.Session {
	sessions := make([]agent.Session, count)
	for index := range sessions {
		sessions[index] = agent.Session{
			ID: fmt.Sprintf("session-%03d", index), Provider: "codex",
			Name: fmt.Sprintf("Task %d", index), WorkingDirectory: fmt.Sprintf("/projects/project-%d", index%projects),
			State: agent.StateRunning,
		}
	}
	return sessions
}

func TestGraphGroupsUseFullPathsAndStableIdentities(t *testing.T) {
	sessions := []agent.Session{
		{ID: "b", Provider: "codex", WorkingDirectory: "/one/app"},
		{ID: "a", Provider: "codex", WorkingDirectory: "/one/app"},
		{ID: "a", Provider: "claude", WorkingDirectory: "/two/app"},
	}
	groups := graphGroups(sessions)
	if len(groups) != 2 || groups[0].indexes[0] != 1 || groups[1].indexes[0] != 2 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestGraphLayoutKeepsEveryNodeWithinItsPage(t *testing.T) {
	for _, width := range []int{40, 60, 90, 160} {
		for _, height := range []int{14, 18, 35} {
			for _, count := range []int{0, 1, 9, 40, 200} {
				sessions := graphTestSessions(count, 5)
				layout := fitGraphAgents(sessions, width, height)
				seen := make(map[int]bool)
				for pageIndex, page := range layout.pages {
					occupied := make(map[[2]int]bool)
					for _, branch := range page.branches {
						for _, node := range branch.nodes {
							if seen[node.index] || node.x < 0 || node.y < 0 || node.x+node.width > width || node.y+node.height > height {
								t.Fatalf("%dx%d count %d: invalid node %+v", width, height, count, node)
							}
							seen[node.index] = true
							if layout.selectedPage(node.index) != pageIndex {
								t.Fatal("selected node is on the wrong page")
							}
							for y := node.y; y < node.y+node.height; y++ {
								for x := node.x; x < node.x+node.width; x++ {
									point := [2]int{x, y}
									if occupied[point] {
										t.Fatalf("overlapping node at %v", point)
									}
									occupied[point] = true
								}
							}
						}
					}
				}
				if len(layout.pages) > 0 && len(seen) != count {
					t.Fatalf("lost nodes: got %d want %d", len(seen), count)
				}
			}
		}
	}
}

func TestGraphAutomaticallyReducesDetailBeforePaging(t *testing.T) {
	cases := []struct{ count, height, rows int }{
		{1, 20, graphFullNodeRows},
		{6, 18, graphCompactNodeRows},
		{12, 29, graphTinyNodeRows},
	}
	for _, test := range cases {
		layout := fitGraphAgents(graphTestSessions(test.count, 1), 100, test.height)
		if layout.nodeRows != test.rows || len(layout.pages) != 1 {
			t.Fatalf("count %d height %d: rows %d pages %d", test.count, test.height, layout.nodeRows, len(layout.pages))
		}
	}
	layout := fitGraphAgents(graphTestSessions(100, 1), 100, 12)
	if len(layout.pages) <= 1 {
		t.Fatal("dense graph should page instead of overlap")
	}
}

func abs(value int) int { return max(value, -value) }
