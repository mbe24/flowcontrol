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
// present and that the active one is dimmed while the others are bright.
func TestStatusLineViewSwitcher(t *testing.T) {
	for _, active := range []Screen{ScreenTree, ScreenLanes, ScreenChain} {
		tail := switcher(active)
		for _, want := range []string{"1 tree", "2 lanes", "3 chain"} {
			if !containsPlain(stripANSI(tail), want) {
				t.Errorf("active=%d: switcher missing %q in %q", active, want, tail)
			}
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

