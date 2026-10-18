package ui

const graphProjectMapGap = 4

// Each column owns its cards and connector lane. Unequal project sizes are
// balanced by height, so a large project cannot overlap its neighbor.
func arrangeProjectMap(cards []graphBranch, width, height int) graphLayout {
	layout := graphLayout{rootX: (width - graphCenterWidth) / 2, rootY: (height - graphRootHeight) / 2}
	if len(cards) == 0 || width < cards[0].width {
		return graphLayout{}
	}
	twoSides := width >= 2*cards[0].width+graphCenterWidth+2*graphProjectMapGap
	if !twoSides {
		maxCardHeight := 0
		for _, card := range cards {
			maxCardHeight = max(maxCardHeight, card.height)
		}
		chainHeight := graphRootHeight + graphVerticalConnectorGap + maxCardHeight
		if chainHeight > height {
			return graphLayout{}
		}
		layout.rootY = (height - chainHeight) / 2
		for _, card := range cards {
			card.x = (width - card.width) / 2
			card.y = layout.rootY + graphRootHeight + graphVerticalConnectorGap
			layout.pages = append(layout.pages, graphPage{branches: []graphBranch{card}})
		}
		return layout
	}
	var columns [2][]graphBranch
	used := [2]int{}
	flush := func() {
		page := graphPage{}
		for side, column := range columns {
			y := (height - max(0, used[side]-graphRowGap)) / 2
			for _, card := range column {
				card.x = layout.rootX - graphProjectMapGap - card.width
				if side == 1 {
					card.x = layout.rootX + graphCenterWidth + graphProjectMapGap
				}
				card.y = y
				y += card.height + graphRowGap
				page.branches = append(page.branches, card)
			}
		}
		if len(page.branches) > 0 {
			layout.pages = append(layout.pages, page)
		}
		columns, used = [2][]graphBranch{}, [2]int{}
	}
	for _, card := range cards {
		if card.height > height {
			return graphLayout{}
		}
		side := 0
		if used[1] < used[0] {
			side = 1
		}
		if used[side]+card.height > height {
			flush()
			side = 0
		}
		columns[side] = append(columns[side], card)
		used[side] += card.height + graphRowGap
	}
	flush()
	return layout
}
