package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestLanesFillWidth guards the lane view taking the full terminal width.
// viewLanes uses fixed lane/gutter sizes, so beyond a few hundred columns the
// frame stops growing and leaves a black margin on the right, unlike every
// other view which fills to the available width (frame == w). Assert the first
// rendered frame line (the top border) is as wide as the view's computed w.
func TestLanesFillWidth(t *testing.T) {
	for _, termW := range []int{80, 100, 120, 160, 200, 240} {
		termW := termW
		t.Run(string(rune('0'+termW/100))+"xx", func(t *testing.T) {
			m := loadModel(t)
			m.width, m.height = termW, 40
			view := m.View()

			first := strings.Split(view, "\n")[0]
			got := runewidth.StringWidth(stripANSI(first))

			// All other views render frame width == w. View() caps even the
			// frame width at 200 (w = min(termW-2, 200)), so the lane view must
			// match that same value — no more early/missing margin.
			want := termW - 2
			if want > 200 {
				want = 200
			}
			if got != want {
				t.Errorf("term width %d: lane frame width = %d, want %d (full width); top border %q",
					termW, got, want, first)
			}
		})
	}
}
