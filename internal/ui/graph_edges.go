package ui

type graphPoint struct{ x, y int }

type graphConnection struct {
	points    []graphPoint
	directory string
}

// Connectors use the reserved gap between columns. They never search for a
// route through another project's band or cross a card.
func graphConnections(page graphPage, root graphNode) []graphConnection {
	var connections []graphConnection
	for _, branch := range page.branches {
		project := graphNode{x: branch.x, y: branch.y, width: branch.width, height: branch.height}
		if len(branch.nodes) == 0 {
			connections = append(connections, graphConnection{points: graphConnector(root, project)})
		} else {
			project = root
		}
		for _, node := range branch.nodes {
			connections = append(connections, graphConnection{
				points: graphConnector(project, node), directory: branch.directory,
			})
		}
	}
	return connections
}

func graphConnector(source, target graphNode) []graphPoint {
	if source.x+source.width/2 == target.x+target.width/2 {
		return graphLine(
			graphPoint{source.x + source.width/2, source.y + source.height - 1},
			graphPoint{target.x + target.width/2, target.y},
		)
	}
	start := graphPoint{source.x + source.width - 1, source.y + source.height/2}
	end := graphPoint{target.x, target.y + target.height/2}
	if target.x < source.x {
		start.x = source.x
		end.x = target.x + target.width - 1
	}
	laneX := (start.x + end.x) / 2
	first := graphPoint{laneX, start.y}
	last := graphPoint{laneX, end.y}
	points := graphLine(start, first)
	points = append(points, graphLine(first, last)[1:]...)
	return append(points, graphLine(last, end)[1:]...)
}

func graphLine(start, end graphPoint) []graphPoint {
	points := []graphPoint{start}
	for start != end {
		switch {
		case start.x < end.x:
			start.x++
		case start.x > end.x:
			start.x--
		case start.y < end.y:
			start.y++
		default:
			start.y--
		}
		points = append(points, start)
	}
	return points
}

type graphStroke struct{ up, down, left, right bool }

func (stroke *graphStroke) connect(point, neighbor graphPoint) {
	stroke.up = stroke.up || neighbor.y < point.y
	stroke.down = stroke.down || neighbor.y > point.y
	stroke.left = stroke.left || neighbor.x < point.x
	stroke.right = stroke.right || neighbor.x > point.x
}

func (stroke graphStroke) glyph() string {
	switch {
	case stroke.up && stroke.down && stroke.left && stroke.right:
		return "┼"
	case stroke.up && stroke.down && stroke.left:
		return "┤"
	case stroke.up && stroke.down && stroke.right:
		return "├"
	case stroke.left && stroke.right && stroke.up:
		return "┴"
	case stroke.left && stroke.right && stroke.down:
		return "┬"
	case stroke.up && stroke.left:
		return "╯"
	case stroke.up && stroke.right:
		return "╰"
	case stroke.down && stroke.left:
		return "╮"
	case stroke.down && stroke.right:
		return "╭"
	case stroke.up || stroke.down:
		return "│"
	default:
		return "─"
	}
}
