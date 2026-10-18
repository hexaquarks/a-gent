package ui

import (
	"sort"

	"a-gent/internal/agent"
)

const (
	graphCenterWidth      = 12
	graphRootHeight       = 3
	graphFullNodeRows     = 6
	graphCompactNodeRows  = 5
	graphTinyNodeRows     = 4
	graphRowGap           = 1
	graphCompactNodeWidth = 16
	graphTinyNodeWidth    = 12
)

type graphGroup struct {
	directory string
	indexes   []int
}

type graphNode struct {
	index               int
	x, y, width, height int
}

type graphBranch struct {
	directory           string
	representativeIndex int
	visibleAgents       int
	x, y, width, height int
	nodes               []graphNode
}

type graphPage struct {
	branches []graphBranch
}

type graphLayout struct {
	pages        []graphPage
	nodeRows     int
	rootX, rootY int
}

// Group by full directory, never its display name. Identity ordering keeps
// nodes still when live timestamps or status change the table's sort order.
func graphGroups(sessions []agent.Session) []graphGroup {
	byDirectory := make(map[string][]int)
	for index, session := range sessions {
		byDirectory[session.WorkingDirectory] = append(byDirectory[session.WorkingDirectory], index)
	}

	directories := make([]string, 0, len(byDirectory))
	for directory := range byDirectory {
		directories = append(directories, directory)
	}
	sort.Strings(directories)

	groups := make([]graphGroup, 0, len(directories))
	for _, directory := range directories {
		indexes := byDirectory[directory]
		sort.SliceStable(indexes, func(i, j int) bool {
			first, second := sessions[indexes[i]], sessions[indexes[j]]
			if first.Provider != second.Provider {
				return first.Provider < second.Provider
			}
			return first.ID < second.ID
		})
		groups = append(groups, graphGroup{directory: directory, indexes: indexes})
	}

	return groups
}

func (layout graphLayout) selectedPage(selected int) int {
	for pageIndex, page := range layout.pages {
		for _, branch := range page.branches {
			if len(branch.nodes) == 0 && branch.representativeIndex == selected {
				return pageIndex
			}
			for _, node := range branch.nodes {
				if node.index == selected {
					return pageIndex
				}
			}
		}
	}
	return 0
}
