package ui

import "strings"

// compactInspectorDiffRows removes shared indentation from a displayed excerpt.
// Relative nesting, diff markers, and provider-recorded line numbers stay intact.
func compactInspectorDiffRows(rows []inspectorDiffRow) []inspectorDiffRow {
	compact := append([]inspectorDiffRow(nil), rows...)
	commonIndent := 0
	foundCode := false

	for index := range compact {
		// Match the preview's existing tab display without changing the source rows.
		compact[index].text = strings.ReplaceAll(compact[index].text, "\t", "  ")
		code := compact[index].text[inspectorDiffMarkerWidth(compact[index].text):]
		if strings.TrimSpace(code) == "" {
			continue
		}

		indent := len(code) - len(strings.TrimLeft(code, " "))
		if !foundCode {
			commonIndent = indent
			foundCode = true
		} else {
			commonIndent = min(commonIndent, indent)
		}
	}

	if commonIndent == 0 {
		return compact
	}

	for index := range compact {
		text := compact[index].text
		markerWidth := inspectorDiffMarkerWidth(text)
		code := text[markerWidth:]
		indent := len(code) - len(strings.TrimLeft(code, " "))
		compact[index].text = text[:markerWidth] + code[min(commonIndent, indent):]
	}
	return compact
}

func inspectorDiffMarkerWidth(text string) int {
	if strings.HasPrefix(text, "+") || strings.HasPrefix(text, "-") || strings.HasPrefix(text, " ") {
		return 1
	}
	return 0
}
