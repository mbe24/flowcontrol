package ui

// Guards around the create dialog: c is not offered on the chain view, and a
// package's parent is the project itself, not whatever row the cursor sits on.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// c must not open the create dialog on the chain view.
func TestUICChainCDoesNotCreate(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenChain

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = upd.(Model)
	assert.Equal(t, OverlayNone, m.overlay, "c on chain must not open create")
}

// c on the tree opens a package create whose parent is the project itself.
func TestUICPackageParentIsProject(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 0 // WP-AUTH row

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = upd.(Model)
	require.Equal(t, OverlayCreate, m.overlay)
	assert.Equal(t, createPackage, m.create.kind)
	assert.Empty(t, m.create.parentID, "a package's parent is the project, not a WP row")
	require.Contains(t, stripANSI(m.viewCreate(80)), "project")
}