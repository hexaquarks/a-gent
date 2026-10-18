package ui

import "a-gent/internal/agent"

const (
	graphAgentCardWidth       = 28
	graphBrowserGap           = 8
	graphVerticalConnectorGap = 3
)

func fitGraphAgents(sessions []agent.Session, width, height int) graphLayout {
	var entries []graphNode
	groups := graphGroups(sessions)
	for _, group := range groups {
		for _, index := range group.indexes {
			entries = append(entries, graphNode{index: index})
		}
	}
	rows, cardWidth := graphFullNodeRows, graphAgentCardWidth
	// Prefer complete titles, then reduce card detail before adding pages.
	sizes := [][2]int{
		{cardWidth, rows},
		{graphCompactNodeWidth, graphCompactNodeRows},
		{graphTinyNodeWidth, graphTinyNodeRows},
	}
	var best graphLayout
	for _, size := range sizes {
		layout := arrangeGraphAgents(entries, sessions, width, height, size[0], size[1])
		if len(layout.pages) == 0 {
			continue
		}
		if len(best.pages) == 0 || len(layout.pages) < len(best.pages) {
			best = layout
		}
		if len(best.pages) == 1 {
			break
		}
	}
	return best
}

func arrangeGraphAgents(
	entries []graphNode, sessions []agent.Session,
	width, height, cardWidth, cardHeight int,
) graphLayout {
	if len(entries) == 0 || width < cardWidth || height < cardHeight {
		return graphLayout{}
	}
	layout := graphLayout{
		nodeRows: cardHeight,
		rootX:    (width - graphCenterWidth) / 2,
		rootY:    (height - graphRootHeight) / 2,
	}
	twoSides := width >= 2*cardWidth+graphCenterWidth+2*graphBrowserGap
	capacity := 1
	if twoSides {
		capacity = 2 * ((height + graphRowGap) / (cardHeight + graphRowGap))
	} else if height < graphRootHeight+graphVerticalConnectorGap+cardHeight {
		return graphLayout{}
	}
	for offset := 0; offset < len(entries); offset += capacity {
		chunk := entries[offset:min(len(entries), offset+capacity)]
		page := graphPage{}
		for index, entry := range chunk {
			entry.width, entry.height = cardWidth, cardHeight
			if twoSides {
				sideCount := (len(chunk) + 1 - index%2) / 2
				columnHeight := sideCount*(cardHeight+graphRowGap) - graphRowGap
				entry.y = (height-columnHeight)/2 + (index/2)*(cardHeight+graphRowGap)
				entry.x = layout.rootX - graphBrowserGap - cardWidth
				if index%2 == 1 {
					entry.x = layout.rootX + graphCenterWidth + graphBrowserGap
				}
			} else {
				layout.rootY = (height - graphRootHeight - graphVerticalConnectorGap - cardHeight) / 2
				entry.x = (width - cardWidth) / 2
				entry.y = layout.rootY + graphRootHeight + graphVerticalConnectorGap
			}
			branch := graphBranch{
				directory: sessions[entry.index].WorkingDirectory,
				nodes:     []graphNode{entry},
			}
			page.branches = append(page.branches, branch)
		}
		layout.pages = append(layout.pages, page)
	}
	return layout
}
