package ui

import "testing"

func TestGraphDirectionPrefersAlignedCardsAndNeverWraps(t *testing.T) {
	current := graphNode{index: 0, x: 50, y: 30, width: 16, height: 5}
	targets := []graphNode{
		current,
		{index: 1, x: 50, y: 0, width: 16, height: 5},
		{index: 2, x: 68, y: 28, width: 16, height: 5},
		{index: 3, x: 50, y: 20, width: 16, height: 5},
		{index: 4, x: 50, y: 40, width: 16, height: 5},
		{index: 5, x: 10, y: 31, width: 16, height: 5},
		{index: 6, x: 90, y: 30, width: 16, height: 5},
	}
	for _, test := range []struct{ horizontal, vertical, want int }{
		{0, -1, 3}, {0, 1, 4}, {-1, 0, 5}, {1, 0, 6},
	} {
		if got := graphDirectionalTarget(current, targets, test.horizontal, test.vertical); got != test.want {
			t.Fatalf("direction (%d,%d) selected %d, want %d", test.horizontal, test.vertical, got, test.want)
		}
	}
	if got := graphDirectionalTarget(targets[1], targets, 0, -1); got != 1 {
		t.Fatal("up at the top should keep the selection")
	}
	if got := graphDirectionalTarget(current, []graphNode{current, targets[2]}, 0, -1); got != 2 {
		t.Fatal("up should use a diagonal card when no aligned card exists")
	}
}
