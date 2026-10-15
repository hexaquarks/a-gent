package ui

import (
	"reflect"
	"testing"
)

func TestCompactInspectorDiffRowsPreservesRelativeNestingAndNumbers(t *testing.T) {
	rows := []inspectorDiffRow{
		{number: 42, numberKnown: true, text: "-            return oldValue"},
		{number: 42, numberKnown: true, text: "+            if ready {"},
		{number: 43, numberKnown: true, text: "+                return newValue"},
		{number: 44, numberKnown: true, text: "+            }"},
		{number: 45, numberKnown: true, text: "             nextStep()"},
	}
	original := append([]inspectorDiffRow(nil), rows...)
	compact := compactInspectorDiffRows(rows)
	expected := []string{"-return oldValue", "+if ready {", "+    return newValue", "+}", " nextStep()"}
	for index, row := range compact {
		if row.text != expected[index] || row.number != rows[index].number || row.numberKnown != rows[index].numberKnown {
			t.Fatalf("row %d = %+v, expected text %q with original number metadata", index, row, expected[index])
		}
	}
	if !reflect.DeepEqual(rows, original) {
		t.Fatal("compacting the visible excerpt changed the cached source rows")
	}
	compact[0].number = 99
	if rows[0].number != 42 {
		t.Fatal("result aliases the input slice")
	}
}

func TestCompactInspectorDiffRowsHandlesBlanksTabsAndFlushLeftCode(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{name: "blank rows do not limit common indent", input: []string{"+", "-  ", "     ", "+        code()", "         next()"},
			expected: []string{"+", "-", " ", "+code()", " next()"}},
		{name: "tabs display as two spaces", input: []string{"-\t\told()", "+\t\tif ready {", "+\t\t\tnew()", " \t\t}"},
			expected: []string{"-old()", "+if ready {", "+  new()", " }"}},
		{name: "flush left prevents trimming", input: []string{"+topLevel()", "-    nested()", "     context()"},
			expected: []string{"+topLevel()", "-    nested()", "     context()"}},
		{name: "only blanks", input: []string{"+", "-  ", " \t"}, expected: []string{"+", "-  ", "   "}},
		{name: "empty excerpt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := make([]inspectorDiffRow, len(test.input))
			for index, text := range test.input {
				rows[index] = inspectorDiffRow{text: text}
			}
			compact := compactInspectorDiffRows(rows)
			if len(compact) != len(test.expected) {
				t.Fatalf("got %d rows, want %d", len(compact), len(test.expected))
			}
			for index, row := range compact {
				if row.text != test.expected[index] {
					t.Fatalf("row %d = %q, want %q", index, row.text, test.expected[index])
				}
			}
		})
	}
}
