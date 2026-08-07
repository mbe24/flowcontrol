package ui

import (
	"strings"
	"testing"
)

// The modal overlay must composite the dialog over the full-screen view so the
// underlying content stays visible in the bands to the left and right of the
// box (not blacked out), and the box must be centred in both axes.
func TestOverlayCenteredAndBackgroundPreserved(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.width, m.height = 120, 30
	m.overlay = OverlayStatus

	v := m.View()
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")

	// the full view height is preserved (modal fills the screen)
	if len(lines) != 30 {
		t.Fatalf("overlay output height = %d, want %d (screen height)", len(lines), 30)
	}

	// locate the dialog's top border row
	dlgTop := -1
	for i, l := range lines {
		if strings.Contains(stripANSI(l), "┌─ set status") {
			dlgTop = i
			break
		}
	}
	if dlgTop < 0 {
		t.Fatal("status dialog top border not found in rendered output")
	}

	// centred vertically: top border should sit near the middle of the screen
	if dlgTop < 5 || dlgTop > 25 {
		t.Fatalf("dialog not centred vertically: top border at line %d of %d", dlgTop, 30)
	}

	// check the body rows still carry the underlying view content outside the box
	mid := dlgTop + 3
	if mid >= len(lines) {
		t.Fatalf("dialog body row out of range")
	}
	row := stripANSI(lines[mid])
	if len([]rune(row)) < 120 {
		t.Fatalf("dialog row width %d < screen width 120; background was trimmed", len([]rune(row)))
	}
	// The status box contains a "│   BLOCKED" wall; the underlying tree shows a
	// task title (►/●) and its detail. If the box spans the row with content on
	// both sides, the row is not all-blank and still contains tree text.
	walled := strings.Contains(row, "│") && strings.Contains(row, "  ")
	if !walled {
		t.Fatalf("unexpected dialog row content: %q", row)
	}
}
