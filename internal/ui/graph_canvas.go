package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	graphDotColumnSpacing = 5
	graphDotRowSpacing    = 3
)

func (canvas *graphCanvas) drawGraphEdges(model Model, page graphPage, root graphNode) {
	strokes := make(map[graphPoint]graphStroke)
	edgeCells := make(map[graphPoint]string)
	edgeColors := make(map[graphPoint]lipgloss.TerminalColor)
	for _, connection := range graphConnections(page, root) {
		color := lipgloss.TerminalColor(lipgloss.Color(colorAccent))
		if connection.directory != "" {
			color = model.projectStyle(connection.directory).GetForeground()
		}
		for index, point := range connection.points {
			stroke := strokes[point]
			if index > 0 {
				stroke.connect(point, connection.points[index-1])
			}
			if index+1 < len(connection.points) {
				stroke.connect(point, connection.points[index+1])
			}
			strokes[point] = stroke
			edgeCells[point] = stroke.glyph()
			edgeColors[point] = color
		}
	}

	// Build each styled edge row once. Repeated ANSI slicing per edge cell
	// duplicates terminal resets and makes dense graphs expensive.
	for y := 0; y < len(canvas.rows)-1; y++ {
		var row, span strings.Builder
		color := lipgloss.TerminalColor(lipgloss.Color(colorDivider))
		for x := 0; x < canvas.width; x++ {
			point := graphPoint{x, y}
			glyph := " "
			nextColor := lipgloss.TerminalColor(lipgloss.Color(colorDivider))
			if x%graphDotColumnSpacing == 0 && y%graphDotRowSpacing == 0 {
				glyph = "·"
			}
			if edge, ok := edgeCells[point]; ok {
				glyph, nextColor = edge, edgeColors[point]
			}
			if nextColor != color {
				row.WriteString(graphEdgeStyle(color).Render(span.String()))
				span.Reset()
				color = nextColor
			}
			span.WriteString(glyph)
		}
		row.WriteString(graphEdgeStyle(color).Render(span.String()))
		canvas.rows[y] = row.String()
	}
}

func graphEdgeStyle(color lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(color).Bold(color != lipgloss.Color(colorDivider))
}
