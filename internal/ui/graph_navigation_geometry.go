package ui

func graphPageTargets(page graphPage) []graphNode {
	var targets []graphNode
	for _, branch := range page.branches {
		if len(branch.nodes) == 0 {
			targets = append(targets, graphNode{
				index: branch.representativeIndex, x: branch.x, y: branch.y, width: branch.width, height: branch.height,
			})
		} else {
			targets = append(targets, branch.nodes...)
		}
	}
	return targets
}

func graphDirectionalTarget(current graphNode, targets []graphNode, horizontal, vertical int) int {
	best := current.index
	const (
		diagonalDirection = iota
		overlappingDirection
		alignedDirection
	)
	bestAlignment, bestDistance := diagonalDirection, 0
	for _, target := range targets {
		// Doubled centers avoid rounding differently for odd and even card sizes.
		dx := 2*target.x + target.width - (2*current.x + current.width)
		dy := 2*target.y + target.height - (2*current.y + current.height)
		if horizontal*dx+vertical*dy <= 0 {
			continue
		}
		overlaps := target.x < current.x+current.width && current.x < target.x+target.width
		sameAxis := dx == 0
		if horizontal != 0 {
			overlaps = target.y < current.y+current.height && current.y < target.y+target.height
			sameAxis = dy == 0
		}
		alignment := diagonalDirection
		if overlaps {
			alignment = overlappingDirection
		}
		if sameAxis {
			alignment = alignedDirection
		}
		// Terminal cells are roughly twice as tall as they are wide.
		const terminalCellHeightRatio = 2
		dy *= terminalCellHeightRatio
		distance := dx*dx + dy*dy
		if best == current.index || alignment > bestAlignment ||
			(alignment == bestAlignment && distance < bestDistance) {
			best, bestAlignment, bestDistance = target.index, alignment, distance
		}
	}
	return best
}
