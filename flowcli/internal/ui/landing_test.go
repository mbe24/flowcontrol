package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// The landing is a screen, not a dialog: it must span the full terminal width
// (not cap at 72), matching the tree/lanes/chain views.
func TestLandingFullWidth(t *testing.T) {
	m := loadModel(t)
	m.width = 160
	m.height = 30

	out := m.viewLanding(160, 30)
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		// wall + space + inner + space + wall = width
		if got := runewidth.StringWidth(stripANSI(ln)); got != 160 {
			t.Errorf("landing line width = %d, want 160: %q", got, stripANSI(ln))
		}
	}
}

// Moving the cursor must not change the height/structure of the list: every
// project is a single stable row. If the percentage "jumps up a row" it means
// the selected row expanded and collapsed the list — this pins it.
func TestLandingRowsStableWhenCursorMoves(t *testing.T) {
	m := loadModel(t)
	m.width = 120
	m.height = 40

	before := m.viewLanding(120, 40)
	beforeLines := strings.Split(strings.TrimRight(before, "\n"), "\n")

	// move cursor down and re-render
	m.landing.cursor = 1
	after := m.viewLanding(120, 40)
	afterLines := strings.Split(strings.TrimRight(after, "\n"), "\n")

	if len(beforeLines) != len(afterLines) {
		t.Fatalf("list height changed on cursor move: before %d lines, after %d",
			len(beforeLines), len(afterLines))
	}

	// the percentage column must occupy the same row relative to each project
	// name across both renders
	beforeSeen, afterSeen := 0, 0
	for i, ln := range beforeLines {
		if strings.Contains(stripANSI(ln), "Travel") {
			beforeSeen = i
			break
		}
	}
	for i, ln := range afterLines {
		if strings.Contains(stripANSI(ln), "Travel") {
			afterSeen = i
			break
		}
	}
	if beforeSeen != afterSeen {
		t.Errorf("project row moved: before line %d, after line %d", beforeSeen, afterSeen)
	}
}

// The memory store fixture has at least the three known projects; the landing
// must list them plus the "+ new project" row.
func TestLandingListsProjectsAndCreateRow(t *testing.T) {
	m := loadModel(t)
	m.width = 120
	m.height = 40

	out := m.viewLanding(120, 40)
	for _, s := range []string{"Travel", "Beer", "Docs", "+ new project"} {
		if !strings.Contains(stripANSI(out), s) {
			t.Errorf("landing missing %q", s)
		}
	}
}
