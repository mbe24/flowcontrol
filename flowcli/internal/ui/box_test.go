package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"flowcli/internal/styles"
)

// truncANSI must cut to the requested display width without leaving an
// unterminated escape sequence or a partial multibyte rune — a bug in here
// would bleed colour past the dialog's right border (the same class of
// regression we fixed on the frame corner before).
func TestTruncANSIWidth(t *testing.T) {
	styled := styles.AccentS.Render("some long title text here")
	for _, w := range []int{0, 5, 10, len("some long title text here")} {
		got := truncANSI(styled, w)
		if gotW := runewidth.StringWidth(stripANSI(got)); gotW != w {
			t.Errorf("truncANSI(%d) width = %d, want %d %q", w, gotW, w, got)
		}
		// colour never bleeds: any escape must be a complete reset by the end
		if strings.Contains(got, "\x1b") && !strings.HasSuffix(got, "\x1b[0m") {
			t.Errorf("truncANSI(%d) left unterminated escape: %q", w, got)
		}
	}
}

// The colour must never "bleed past" the border. If a truncated line still
// carries a foreground colour, the border wall that follows it would inherit
// it. So a truncated styled string must end colour-neutral.
func TestTruncANSIEndsColourNeutral(t *testing.T) {
	styled := styles.AccentS.Render("abcdefghij")
	got := truncANSI(styled, 6)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("expected reset suffix, got %q", got)
	}
	// strip it and confirm exactly 6 visible cells survived
	if w := runewidth.StringWidth(stripANSI(got)); w != 6 {
		t.Errorf("visible width = %d, want 6", w)
	}
}

func TestTruncANSINoBleed(t *testing.T) {
	raw := "a very long line that must be cut cleanly"
	styled := styles.BrightS.Render(raw)
	got := truncANSI(styled, 10)
	if w := runewidth.StringWidth(stripANSI(got)); w != 10 {
		t.Errorf("width = %d, want 10", w)
	}
	if strings.Index(stripANSI(got), "must") != -1 {
		t.Errorf("truncation kept text past the cut: %q", got)
	}
}

// A long body line that exceeds the inner width is truncated, so every
// rendered row is exactly `inner` wide (plus the two walls). This is what
// keeps the dialog's right border a straight line.
func TestBoxWithKeysTruncatesLongRows(t *testing.T) {
	inner := 20
	out := boxWithKeys("test", styles.Accent,
		[]string{"this body line is much longer than twenty columns", "ok"},
		"↵ ok", inner, 80)

	lines := strings.Split(out, "\n")
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		if w := runewidth.StringWidth(stripANSI(ln)); w != inner+4 {
			t.Errorf("line %d width = %d, want %d: %q", i, w, inner+4, ln)
		}
	}
}

// The top border's "┐" corner must land exactly at the right wall — the
// header dashes are sized from display width, not byte length.
func TestBoxWithKeysCornerAligns(t *testing.T) {
	inner := 40
	out := boxWithKeys("new task", styles.Accent, []string{"title: rotate token"}, "↵ create", inner, 100)
	lines := strings.Split(out, "\n")

	// the header is the first non-empty line: ╭─ new task ──...─╮
	head := ""
	for _, ln := range lines {
		if strings.HasPrefix(stripANSI(ln), "╭") {
			head = stripANSI(ln)
			break
		}
	}
	if !strings.HasSuffix(head, "╮") {
		t.Fatalf("header does not end with corner: %q", head)
	}
	// every row (header included) must be the same display width
	want := inner + 4
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		got := runewidth.StringWidth(stripANSI(ln))
		if got != want {
			t.Errorf("row %d width = %d, want %d: %q", i, got, want, stripANSI(ln))
		}
	}
}

// The key line and body rows must all share the same inner width so the two
// walls are straight on both sides.
func TestBoxWithKeysRowsAligned(t *testing.T) {
	inner := 30
	out := boxWithKeys("title", styles.Accent,
		[]string{"short", "another line"}, // different lengths
		"^n another  ↵ create", inner, 80)
	lines := strings.Split(out, "\n")
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		if got := runewidth.StringWidth(stripANSI(ln)); got != inner+4 {
			t.Errorf("row %d width = %d, want %d", i, got, inner+4)
		}
	}
}

// boxWithKeys must clamp inner to the terminal width so the dialog never
// overflows a narrow terminal.
func TestBoxWithKeysClampsToTerminal(t *testing.T) {
	// inner=100 would exceed a 60-col terminal → clamped to 54
	out := boxWithKeys("t", styles.Accent, []string{"x"}, "", 100, 60)
	lines := strings.Split(out, "\n")
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		if got := runewidth.StringWidth(stripANSI(ln)); got > 58 {
			t.Errorf("dialog exceeds terminal: width %d (%q)", got, stripANSI(ln))
		}
	}
}
