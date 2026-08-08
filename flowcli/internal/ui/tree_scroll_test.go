package ui

import (
	"strings"
	"testing"
)

// small-narrow model forces the tree to scroll: many rows in a short window.
func treeModel(t *testing.T, height int) Model {
	t.Helper()
	m := loadModel(t)
	m.screen = ScreenTree
	m.width = 120
	m.height = height
	return m
}

// TestTreeSelectionVisible verifies the selected row is actually rendered in
// the tree body (not scrolled out and not swallowed), so the selection state
// is observable in the rendered window.
func TestTreeSelectionVisible(t *testing.T) {
	m := treeModel(t, 40)
	out := m.View()
	sel := stripANSI(m.rows[m.cursor].node.Title)
	plain := stripANSI(out)
	if !strings.Contains(plain, sel) {
		t.Errorf("selected row title %q not visible in rendered tree; view:\n%s",
			sel, out)
	}
}

// TestSelectedLineStripsInternalResets verifies the selection logic is not
// defeated by per-segment colour resets. selectedLine removes the intermediate
// SGR resets from an already-styled row so a single background can span the
// whole line, while keeping the row's full visible width. (Background colour
// itself is TTY-dependent and can't be asserted under `go test`, so we check
// the reset-stripping and width-preservation properties instead.)
func TestSelectedLineSpansRow(t *testing.T) {
	inner := 30
	// an already-styled row with per-segment resets (like treeRow produces)
	content := "  " + "\x1b[38;5;1m" + "●" + "\x1b[0m" + " " + "\x1b[38;5;15m" + "T-1042" + "\x1b[0m"
	line := content + strings.Repeat(" ", inner-wlen(content))
	out := selectedLine(line)

	// No intermediate resets may remain: otherwise a background read across the
	// whole row would be cut off at the first one.
	if strings.Contains(out[:len(out)-len("\x1b[0m")], "\x1b[0m") {
		t.Errorf("selectedLine left an internal reset; got %q", out)
	}
	if got := wlen(out); got != inner {
		t.Errorf("selectedLine width = %d, want %d (pad spans full row)", got, inner)
	}
	if !strings.Contains(stripANSI(out), "T-1042") {
		t.Errorf("selectedLine dropped the row content; got %q", out)
	}
}

// TestTreeScrollsToCursor verifies the tree's vertical scroll follows the
// cursor: moving down past the visible window advances treeScroll so the
// selected row stays on screen. Regression for "scrolling does not work in
// tree view".
func TestTreeScrollsToCursor(t *testing.T) {
	m := treeModel(t, 10) // tiny height -> only a couple rows visible
	win := m.treeVisibleRows()
	if win < 1 {
		t.Fatalf("treeVisibleRows()=%d, want >= 1", win)
	}
	// sanity: there must be more rows than fit in the window
	if len(m.rows) <= win {
		t.Skipf("only %d rows; nothing to scroll", len(m.rows))
	}

	// Move the cursor far enough down to force scrolling.
	steps := len(m.rows)
	for i := 0; i < steps; i++ {
		m = press(t, m, "j")
	}
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("cursor=%d, want last row %d", m.cursor, len(m.rows)-1)
	}
	if m.treeScroll == 0 {
		t.Errorf("treeScroll=0 after moving to last row; expected > 0 (no scrolling)")
	}
	if m.cursor < m.treeScroll || m.cursor >= m.treeScroll+m.treeVisibleRows() {
		t.Errorf("cursor=%d outside visible window [%d,%d)",
			m.cursor, m.treeScroll, m.treeScroll+m.treeVisibleRows())
	}

	// The selected (last) row's ID and title must actually appear in the
	// rendered output — not just be "in the window" but then truncated by
	// frame() because of blocker sub-lines.
	out := stripANSI(m.View())
	if !strings.Contains(out, m.rows[m.cursor].node.ID) {
		t.Errorf("selected row ID %q not visible after scrolling to bottom",
			m.rows[m.cursor].node.ID)
	}
	if !strings.Contains(out, stripANSI(m.rows[m.cursor].node.Title)) {
		t.Errorf("selected row title %q not visible after scrolling to bottom",
			m.rows[m.cursor].node.Title)
	}

	// Scrolling back up restores the top.
	m = treeModel(t, 10)
	for i := 0; i < win+5; i++ {
		m = press(t, m, "j")
	}
	if m.treeScroll > 0 {
		for i := 0; i < len(m.rows); i++ {
			m = press(t, m, "k")
		}
		if m.treeScroll != 0 {
			t.Errorf("after scrolling back up, treeScroll=%d, want 0", m.treeScroll)
		}
		if m.cursor != 0 {
			t.Errorf("after scrolling back up, cursor=%d, want 0", m.cursor)
		}
	}
}
