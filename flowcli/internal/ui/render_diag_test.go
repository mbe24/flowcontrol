package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"flowcli/internal/store"
)

func dump(t *testing.T, w, h int, m Model, label string) {
	t.Helper()
	m.width, m.height = w, h
	view := m.View()
	ruler := strings.Repeat("0123456789", (w/10)+1)[:w]
	t.Logf("=== %s (w=%d) ===", label, w)
	t.Logf("COLUMN: %s", ruler)
	for i, line := range strings.Split(view, "\n") {
		plain := stripANSI(line)
		cw := runewidth.StringWidth(plain)
		var clipped string
		if len(plain) > 60 {
			clipped = string([]rune(plain)[:60]) + "…"
		} else {
			clipped = plain
		}
		t.Logf("%2d [%3d] %q", i, cw, clipped)
	}
}

func loadModel(t *testing.T) Model {
	t.Helper()
	m := New(store.NewMemory())
	msg := m.load().(loadedMsg)
	upd, _ := m.Update(msg)
	m = upd.(Model)
	m.screen = ScreenLanes
	return m
}

func TestDiag_TreeHeader(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	dump(t, 100, 40, m, "tree")
}

func TestDiag_Lanes(t *testing.T) {
	m := loadModel(t)
	dump(t, 100, 40, m, "lanes-100")
}
