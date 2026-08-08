package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"flowcli/internal/store"
)

// The status dialog rows are fixed-width and each carries a right-aligned
// shortcut letter (r/b/x/d) in the status colour; the target node's current
// status is annotated with a grey "(current)".
func TestStatusOverlayShowsCurrentAndShortcuts(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.cursor = 1 // T-1042 (READY)
	m.selectedID = "T-1042"
	m.overlay = OverlayStatus

	v := stripANSI(m.View())
	if !strings.Contains(v, "(current)") {
		t.Errorf("expected a grey (current) marker on the target's current status; got:\n%s", v)
	}
	// shortcuts are rendered for every status row
	for _, key := range []string{"r", "b", "x", "d"} {
		if !strings.Contains(v, key) {
			t.Errorf("expected status shortcut %q in status dialog; got:\n%s", key, v)
		}
	}
}

// Pressing r/b/x/d in the status overlay dispatches the matching status write
// through the same cascade path as Enter (no cascade here, so it applies
// immediately).
func TestStatusShortcutDispatchesStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // T-1042
	m.selectedID = "T-1042"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	require.Equal(t, OverlayStatus, m.overlay)

	// T-1042 is READY; pressing d (Done) applies directly (no deps, no open steps)
	ms.On("SetStatus", mock.Anything, "T-1042", store.Done).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	require.Contains(t, m.flash, "nothing was waiting")
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// The shortcut path still routes into the cascade preview when the change has
// downstream effects.
func TestStatusShortcutOpensCascade(t *testing.T) {
	ms := new(mockStore)
	m := buildCascadeModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // T-1 (has an open step + blocked dependent)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	require.Equal(t, OverlayStatus, m.overlay)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = upd.(Model)
	require.Equal(t, OverlayCascade, m.overlay)
	require.True(t, m.cascade.active)
}
