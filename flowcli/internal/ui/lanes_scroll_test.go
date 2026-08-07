package ui

import (
	"strings"
	"testing"

	"flowcli/internal/store"
)

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

	// move to the last READY card
	ready := m.laneTasks(store.Ready)
	last := len(ready) - 1
	for steps := 0; steps <= last; steps++ {
		m = press(t, m, "j")
	}
	if m.laneCursor[0] != last {
		t.Fatalf("expected cursor at last card %d, got %d", last, m.laneCursor[0])
	}

	view := m.View()
	lines := strings.Split(view, "\n")

	// header + rule should always be the fixed top two body rows
	if !strings.Contains(lines[1], "● READY") {
		t.Errorf("expected READY header on line 1, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "──") {
		t.Errorf("expected rule on line 2, got %q", lines[2])
	}

	// the selected card (▸ mark next to its ID) must be visible in the body
	sel := m.ownerTask(ready[last]).ID
	found := false
	for _, ln := range lines[3:] {
		if strings.Contains(ln, "▸"+sel) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("selected card %s not visible in rendered view:\n%s", sel, view)
	}
}
