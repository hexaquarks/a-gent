package ui

import "testing"

func TestGraphProjectAndAgentCardsNeverOverlapTheHubOrEachOther(t *testing.T) {
	for _, size := range [][2]int{{90, 20}, {125, 40}} {
		layout := graphProjectLayoutForTest(40, 7, size[0], size[1])
		for _, page := range layout.pages {
			cards := []graphNode{{x: layout.rootX, y: layout.rootY, width: graphCenterWidth, height: graphRootHeight}}
			for _, branch := range page.branches {
				cards = append(cards, graphNode{x: branch.x, y: branch.y, width: branch.width, height: branch.height})
				cards = append(cards, branch.nodes...)
			}
			occupied := make(map[graphPoint]bool)
			for _, card := range cards {
				for y := card.y; y < card.y+card.height; y++ {
					for x := card.x; x < card.x+card.width; x++ {
						point := graphPoint{x, y}
						if occupied[point] || x < 0 || x >= size[0] || y < 0 || y >= size[1] {
							t.Fatalf("overlapping or clipped card at %+v in %v", point, size)
						}
						occupied[point] = true
					}
				}
			}
		}
	}
}

// Colored agent connectors must never enter another project's connector lane.
// Shared hub trunks are intentional and use the hub's color.
func TestGraphProjectConnectionsStaySeparate(t *testing.T) {
	for _, count := range []int{9, 15, 40, 100} {
		layout := graphProjectLayoutForTest(count, 5, 125, 40)
		root := graphNode{x: layout.rootX, y: layout.rootY, width: graphCenterWidth, height: graphRootHeight}
		for _, page := range layout.pages {
			owners := make(map[graphPoint]string)
			for _, connection := range graphConnections(page, root) {
				if connection.directory == "" {
					continue
				}
				for _, point := range connection.points {
					if owner, exists := owners[point]; exists && owner != connection.directory {
						t.Fatalf("project connectors cross at %+v: %s and %s", point, owner, connection.directory)
					}
					owners[point] = connection.directory
				}
			}
		}
	}
}
