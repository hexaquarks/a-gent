package ui

import "testing"

func TestGraphRoutesEveryEdgeAroundAllCards(t *testing.T) {
	for _, count := range []int{1, 9, 15, 40} {
		for _, size := range [][2]int{{90, 20}, {125, 40}} {
			layout := fitGraphAgents(graphTestSessions(count, 5), size[0], size[1])
			root := graphNode{x: layout.rootX, y: layout.rootY, width: graphCenterWidth, height: graphRootHeight}
			for _, page := range layout.pages {
				rectangles := []graphNode{root}
				for _, branch := range page.branches {
					if branch.height > 0 {
						rectangles = append(rectangles, graphNode{x: branch.x, y: branch.y, width: branch.width, height: branch.height})
					}
					rectangles = append(rectangles, branch.nodes...)
				}
				for _, connection := range graphConnections(page, root) {
					for index, point := range connection.points {
						if index > 0 && index < len(connection.points)-1 {
							for _, card := range rectangles {
								if point.x >= card.x && point.x < card.x+card.width &&
									point.y >= card.y && point.y < card.y+card.height {
									t.Fatalf("connector crossed a card at %+v", point)
								}
							}
						}
						if index > 0 {
							previous := connection.points[index-1]
							if abs(point.x-previous.x)+abs(point.y-previous.y) != 1 {
								t.Fatal("edge has a gap")
							}
						}
					}
				}
			}
		}
	}
}
