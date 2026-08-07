package ui

import (
	"strings"
	"testing"

	"flowcli/internal/store"
)

// visibleIDs returns the set of lane-card task IDs currently visible in the
// rendered lane view (between the rule row and the separator).
func visibleIDs(t *testing.T, m Model) map[string]bool {
	t.Helper()
	view := m.View()
	lines := strings.Split(view, "\n")
	ids := map[string]bool{}
	for _, ln := range lines {
		for _, tok := range strings.Fields(stripANSI(ln)) {
			id := strings.TrimLeft(tok, "▸ ")
			if strings.HasPrefix(id, "T-") && len(id) > 2 {
				ids[id] = true
			}
		}
	}
	return ids
}

// TestLanesVerticalScroll verifies that as the cursor moves down a lane, the
// rendered lane view scrolls so the selected card stays visible, while the
// header and rule rows stay pinned to the top. This guards against the
// regression where the selection moved below the fold but the view stayed
// pinned to the top, making lower cards inaccessible.
func TestLanesVerticalScroll(t *testing.T) {
	// width 100, height 14: only ~6 grid rows fit, but READY has 5 cards
	// (~29 grid rows) so scrolling must engage.
	m := loadModel(t)
	m.width, m.height = 100, 14
	m.screen = ScreenLanes
	m.lane = 0 // READY
	m.laneCursor[0] = 0

	// move to the last READY card via repeated j presses
	ready := m.laneTasks(store.Ready)
	last := len(ready) - 1
	if last < 1 {
		t.Fatalf("need >1 READY cards to test scrolling, have %d", len(ready))
	}

	topIDs := visibleIDs(t, m)
	// scroll down step by step and require that each selected card becomes
	// visible (the whole point of the fix)
	for steps := 0; steps <= last; steps++ {
		m = press(t, m, "j")
		if m.laneCursor[0] != min(steps+1, last) {
			t.Fatalf("cursor did not advance: want %d got %d", min(steps+1, last), m.laneCursor[0])
		}
		sel := m.ownerTask(ready[m.laneCursor[0]]).ID
		got := visibleIDs(t, m)
		if !got[sel] {
			t.Errorf("after moving to card %d, selected %s not visible (top=%v got=%v)",
				m.laneCursor[0], sel, topIDs, got)
		}
	}

	// header + rule should always be the fixed top two body rows
	view := m.View()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[1], "● READY") {
		t.Errorf("expected READY header on line 1, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "──") {
		t.Errorf("expected rule on line 2, got %q", lines[2])
	}

	// the selection must be visible at the end
	sel := m.ownerTask(ready[last]).ID
	ids := visibleIDs(t, m)
	if !ids[sel] {
		t.Errorf("last selected card %s not visible in view:\n%s", sel, view)
	}
}
