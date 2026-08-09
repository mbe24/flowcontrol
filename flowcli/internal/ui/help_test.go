package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/help"
)

// TestStatusLineWidth is a regression guard for the status/help row: every
// screen's statusLine must be exactly `inner` cells wide so the frame's right
// wall closes (a broken right-aligned switcher or over-long help would let the
// corner fall short). It also verifies the 1/2/3 view switcher is present.
func TestStatusLineWidth(t *testing.T) {
	m := Model{help: help.New()}
	for _, tc := range []struct {
		screen Screen
		inner  int
	}{
		{ScreenTree, 96},
		{ScreenLanes, 60},
		{ScreenChain, 96},
		{ScreenDetail, 96},
	} {
		km := kmFor(tc.screen)
		line := m.statusLine(km, tc.screen, tc.inner)
		if got := wlen(line); got != tc.inner {
			t.Errorf("screen %d: statusLine width %d != inner %d\nline=%q", tc.screen, got, tc.inner, line)
		}
	}
}

// TestStatusLineViewSwitcher checks that all three view-switch hints are always
// present, in ascending order (1, 2, 3), and that the active one is dimmed
// while the others are bright.
func TestStatusLineViewSwitcher(t *testing.T) {
	for _, active := range []Screen{ScreenTree, ScreenLanes, ScreenChain} {
		tail := switcher(active)
		plain := stripANSI(tail)
		// order: 1 tree ... 2 lanes ... 3 chain
		one := strings.Index(plain, "1 tree")
		two := strings.Index(plain, "2 lanes")
		three := strings.Index(plain, "3 chain")
		if one < 0 || two < 0 || three < 0 {
			t.Fatalf("active=%d: switcher missing labels in %q", active, plain)
		}
		if !(one < two && two < three) {
			t.Errorf("active=%d: switcher not in 1,2,3 order: %q", active, plain)
		}
		for _, want := range []string{"1 tree", "2 lanes", "3 chain"} {
			if !containsPlain(tail, want) {
				t.Errorf("active=%d: switcher missing %q in %q", active, want, tail)
			}
		}
	}
}

// TestHelpOverlayRenders checks that the "?" full-help is a full-screen panel
// titled "key map" with three column groups (MOVE, ACT, FIND & SCOPE). A
// regression guard so the help panel keeps rendering and isn't empty.
func TestHelpOverlayRenders(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.overlay = OverlayHelp
	m.width, m.height = 100, 20
	ov := m.View()
	if strings.TrimSpace(ov) == "" {
		t.Fatal("? help panel rendered empty")
	}
	for _, probe := range []string{"MOVE", "ACT", "FIND & SCOPE", "move", "find", "quit"} {
		if !containsPlain(stripANSI(ov), probe) {
			t.Errorf("? help panel missing %q in:\n%s", probe, stripANSI(ov))
		}
	}
	// The panel is borderless and flush to the top: the first line must be a
	// lane title, with no frame corners or walls around the content.
	plain := stripANSI(ov)
	firstLine := strings.SplitN(plain, "\n", 2)[0]
	if !strings.Contains(firstLine, "MOVE") {
		t.Errorf("? help panel not flush to top; first line = %q", firstLine)
	}
	for _, marker := range []string{"╭", "╮", "╰", "╯", "│", "├"} {
		if strings.Contains(plain, marker) {
			t.Errorf("? help panel should be borderless but contains %q", marker)
		}
	}
}

// The tree keymap exposes both O (new child) and o (new sibling), and the
// help panel lists them in the ACT lane.
func TestHelpKeymapSibling(t *testing.T) {
	km := kmFor(ScreenTree)
	if !km.Child.Enabled() || !km.Sibling.Enabled() {
		t.Fatal("tree keymap must bind O (child) and o (sibling)")
	}
	if !containsPlain(km.Sibling.Help().Key, "o") || !containsPlain(km.Sibling.Help().Desc, "sibling") {
		t.Fatalf("sibling binding wrong: %q", km.Sibling.Help())
	}

	m := loadModel(t)
	m.screen = ScreenTree
	m.overlay = OverlayHelp
	m.width, m.height = 100, 20
	plain := stripANSI(m.View())
	for _, probe := range []string{"new child", "new sibling"} {
		if !containsPlain(plain, probe) {
			t.Errorf("? help panel missing %q", probe)
		}
	}
}


func kmFor(s Screen) screenKeys {
	switch s {
	case ScreenLanes:
		return lanesKeys()
	case ScreenChain:
		return chainKeys()
	case ScreenDetail:
		return detailKeys()
	default:
		return treeKeys()
	}
}

func containsPlain(s, sub string) bool { return strings.Contains(s, sub) }

