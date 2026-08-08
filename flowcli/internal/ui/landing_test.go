package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// splitLines splits on newlines (kept simple; the landing has no \r).
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// slashCol / pctCol report the display column (runewidth) of the first '/'
// and '%' in a possibly-ANSI line. Byte offsets would be wrong for multi-byte
// glyphs (e.g. the ❯ cursor), so alignment checks use display columns.
func slashCol(s string) int {
	w := 0
	for _, r := range []rune(stripANSI(s)) {
		if r == '/' {
			return w
		}
		w += runewidth.RuneWidth(r)
	}
	return -1
}

func pctCol(s string) int {
	w := 0
	for _, r := range []rune(stripANSI(s)) {
		if r == '%' {
			return w
		}
		w += runewidth.RuneWidth(r)
	}
	return -1
}

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

// Selecting a project on the landing must land on the TREE view (1), not pop
// open the first work-package's detail view. Regression for: "when I select a
// project, I should be at its tree view (1) but I'm on some empty page with
// the title 'WP-AUTH -- Authentication Infrastructure'".
func TestLandingEnterOpensTree(t *testing.T) {
	m := loadModel(t)
	m.width = 120
	m.height = 40
	// boot state: on the landing screen
	m.screen = ScreenLanding
	// load projects into the model
	msg := m.load().(loadedMsg)
	upd, _ := m.Update(msg)
	m = upd.(Model)

	m = press(t, m, "enter")
	if m.screen != ScreenTree {
		t.Fatalf("after selecting project: screen=%v, want ScreenTree (%v)",
			m.screen, ScreenTree)
	}
	if m.projectID == "" {
		t.Errorf("expected projectID to be set after selecting a project")
	}
	if m.overlay != OverlayNone {
		t.Errorf("expected no overlay after selecting, got %v", m.overlay)
	}
}

// The landing must render real progress (done/total + percent), not a dead
// 0/0: a project with seeded subtasks must show a non-zero ratio. This pins
// the earlier regression where landing.counts was never populated.
func TestLandingShowsProjectProgress(t *testing.T) {
	m := loadModel(t)
	m.width = 120
	m.height = 40

	out := stripANSI(m.viewLanding(120, 40))
	if strings.Contains(out, "0/0") {
		t.Errorf("landing shows 0/0 progress; expected populated ratios: %q",
			out)
	}
}

// The ratio's slash and the percent must line up in shared columns so the
// landing reads as a tidy table even though names differ in length. Regression
// for: "the steps are not aligned and the percentages are not aligned".
// Positions are measured in display columns (runewidth) — byte offsets would
// mis-report the multi-byte ❯ cursor.
func TestLandingRatioAndPercentAligned(t *testing.T) {
	m := loadModel(t)
	m.width = 120
	m.height = 40

	out := m.viewLanding(120, 40)
	type col struct{ slash, pct int }
	var cols []col
	for _, ln := range splitLines(out) {
		plain := stripANSI(ln)
		if !strings.Contains(plain, "/") || !strings.Contains(plain, "%") {
			continue
		}
		cols = append(cols, col{slashCol(ln), pctCol(ln)})
	}
	if len(cols) < 2 {
		t.Fatalf("expected at least 2 project rows, found %d", len(cols))
	}
	first := cols[0]
	for i, c := range cols[1:] {
		if c.slash != first.slash {
			t.Errorf("row %d slash at display col %d, want %d (ratios not aligned)",
				i+1, c.slash, first.slash)
		}
		if c.pct != first.pct {
			t.Errorf("row %d percent %% at display col %d, want %d (percents not aligned)",
				i+1, c.pct, first.pct)
		}
	}
}

