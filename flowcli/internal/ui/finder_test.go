package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The find dialog must keep a constant height regardless of how many results
// match, and the current selection must always be visible within it. This
// guards against the dialog growing/shrinking with the result set.
func TestFinderFixedHeight(t *testing.T) {
	m := loadModel(t)
	m.overlay = OverlayFinder
	m.width, m.height = 120, 40
	m.input.SetValue("t")
	m.finderHits = m.search("t")
	if len(m.finderHits) == 0 {
		t.Fatal("expected some hits for query 't'")
	}

	// grow well past one page by duplicating a probe node
	probe := m.finderHits[0]
	for len(m.finderHits) < finderVisible*3 {
		m.finderHits = append(m.finderHits, probe)
	}
	t.Logf("simulated hits: %d (visible=%d)", len(m.finderHits), finderVisible)

	// height (number of box lines) must not depend on the number of hits
	hMany := len(strings.Split(m.viewOverlay(m.width), "\n"))
	wMany := topWidth(m.viewOverlay(m.width))

	m.finderHits = m.search("zzz-no-such-query")
	hFew := len(strings.Split(m.viewOverlay(m.width), "\n"))

	if hMany != hFew {
		t.Fatalf("find dialog height depends on results: many=%d few=%d (want equal)", hMany, hFew)
	}

	// width (longest box line) must also be constant regardless of the number
	// of results, per the fixed-size requirement.
	wFew := topWidth(m.viewOverlay(m.width))
	if wMany != wFew {
		t.Fatalf("find dialog width changed: many=%d few=%d (want equal)", wMany, wFew)
	}
}

// topWidth returns the display width of the first (border) line of a box.
func topWidth(ov string) int {
	first := strings.Split(ov, "\n")[0]
	return wlen(first)
}

// Scrolling deeper than the visible window must keep the selected row inside
// the rendered dialog and advance finderScroll.
func TestFinderScrollKeepsSelection(t *testing.T) {
	m := loadModel(t)
	m.overlay = OverlayFinder
	m.width, m.height = 120, 40
	m.input.SetValue("t")
	hits := m.search("t")
	probe := hits[0]
	for len(hits) < finderVisible*3 {
		hits = append(hits, probe)
	}
	m.finderHits = hits

	// drive Update directly with a down key past one page
	for i := 0; i < finderVisible+2; i++ {
		upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	in := m.finderIdx >= m.finderScroll && m.finderIdx < m.finderScroll+finderVisible
	if !in {
		t.Fatalf("selection (idx=%d) fell outside visible window [%d,%d)",
			m.finderIdx, m.finderScroll, m.finderScroll+finderVisible)
	}
	// moving up must restore the window so the selection is visible again
	for i := 0; i < finderVisible+2; i++ {
		upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = upd.(Model)
	}
	if m.finderIdx != 0 {
		t.Fatalf("moving up to top: idx=%d want 0", m.finderIdx)
	}
	if m.finderScroll != 0 {
		t.Fatalf("moving up to top: scroll=%d want 0", m.finderScroll)
	}
}

// Opening a task from the finder and pressing ESC must return to the finder
// with the same search query intact, so the user can continue browsing.
func TestFinderEscReturnsToFinder(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.cursor = 0
	m.selectedID = ""
	if len(m.rows) == 0 {
		t.Skip("no tree rows in fixture")
	}

	// open the finder from the tree with a query
	m = press(t, m, "/")
	m = press(t, m, "t")
	if m.overlay != OverlayFinder {
		t.Fatalf("expected finder overlay, got %v", m.overlay)
	}
	q := m.input.Value()
	if q == "" || len(m.finderHits) == 0 {
		t.Skip("query 't' produced no finder hits in fixture")
	}

	// open the selected task from the finder
	selBefore := m.finderIdx
	m = press(t, m, "enter")
	if m.screen != ScreenDetail {
		t.Fatalf("after enter from finder: expected detail, got %v", m.screen)
	}

	// ESC must return to the finder, not the tree
	m = press(t, m, "esc")
	if m.screen == ScreenDetail {
		t.Fatalf("after esc: still in detail")
	}
	if m.overlay != OverlayFinder {
		t.Fatalf("after esc: expected finder overlay, got %v", m.overlay)
	}
	if m.input.Value() != q {
		t.Errorf("after esc: query changed, got %q want %q", m.input.Value(), q)
	}
	if m.finderIdx != selBefore {
		t.Errorf("after esc: selection changed, got idx=%d want %d", m.finderIdx, selBefore)
	}
}

// Opening a task from the tree (not the finder) and pressing ESC must return to
// the tree, not reopen any finder.
func TestFinderNotOpenedFromNormalEnter(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.cursor = 0
	m.selectedID = ""
	if len(m.rows) == 0 {
		t.Skip("no tree rows in fixture")
	}
	m.selectedID = m.ownerTask(m.rows[0].node).ID

	m = press(t, m, "enter")
	if m.screen != ScreenDetail {
		t.Fatalf("expected detail, got %v", m.screen)
	}
	m = press(t, m, "esc")
	if m.overlay != OverlayNone {
		t.Errorf("after esc from normal detail: expected no overlay, got %v", m.overlay)
	}
	if m.screen != ScreenTree {
		t.Errorf("after esc from normal detail: expected tree, got %v", m.screen)
	}
}
