package ui

// Inline edit (c in detail) dispatches UpdateNode with only the changed
// fields, shows the stale note on condition change, and rejects empty titles.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"flowcli/internal/store"
)

// openDetailEdit drives the model into detail view and presses c.
func openDetailEdit(t *testing.T, ms *mockStore) Model {
	t.Helper()
	m := loadMockedModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1042"
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	return upd.(Model)
}

// c in detail opens the edit dialog prefilled with the node's title/condition.
func TestUIEditOpensDialog(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	require.Equal(t, OverlayEdit, m.overlay, "c in detail must open the edit overlay")
	require.Equal(t, "T-1042", m.edit.nodeID)
	require.Equal(t, "Device-code flow", m.edit.form.Value(0))
	require.Equal(t, "pnpm test:auth --grep device", m.edit.form.Value(1))
}

// esc closes the edit dialog without touching the store.
func TestUIEditEscCloses(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m = upd.(Model)
	assert.Equal(t, OverlayNone, m.overlay)
	ms.AssertNotCalled(t, "UpdateNode", mock.Anything, mock.Anything, mock.Anything)
}

// Editing the title dispatches UpdateNode with only the Title field set.
func TestUIEditSavesTitle(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	// edit the title field (the first one); the focused field is title.
	// textinput starts prefilled, so clear before typing (ctrl+u deletes
	// everything before the cursor).
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = upd.(Model)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("New flow")})
	m = upd.(Model)

	wantTitle := "New flow"
	ms.On("UpdateNode", mock.Anything, "T-1042",
		store.NodeUpdate{Title: &wantTitle}).Return(nil).Once()

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// Editing only the condition keeps the title pointer nil and shows the stale
// note in the dialog.
func TestUIEditSavesCondition(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	// move focus to the condition field
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	// clear the prefilled condition, then type a fresh one
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = upd.(Model)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pnpm test:new --grep")})
	m = upd.(Model)

	// the stale note shows once the condition differs from the loaded value.
	// The dialog truncates to its inner width, so assert the visible prefix.
	view := m.viewEdit(80)
	assert.Contains(t, stripANSI(view), "! editing the condition marks")

	wantCond := "pnpm test:new --grep"
	ms.On("UpdateNode", mock.Anything, "T-1042",
		store.NodeUpdate{Condition: &wantCond}).Return(nil).Once()

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// Ctrl+S is an alternative submit binding.
func TestUIEditSavesWithCtrlS(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	// clear the prefilled title, then type a fresh one
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = upd.(Model)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("New flow")})
	m = upd.(Model)
	wantTitle := "New flow"
	ms.On("UpdateNode", mock.Anything, "T-1042",
		store.NodeUpdate{Title: &wantTitle}).Return(nil).Once()

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// Submitting without changes must not call the store.
func TestUIEditNoChangeNoCall(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	// title is prefilled with the node's current title; don't type anything
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)

	assert.Equal(t, OverlayNone, m.overlay, "unchanged edit should just close")
	ms.AssertNotCalled(t, "UpdateNode", mock.Anything, mock.Anything, mock.Anything)
}

// An empty title must be rejected locally, without a store call.
func TestUIEditEmptyTitleRejects(t *testing.T) {
	ms := new(mockStore)
	m := openDetailEdit(t, ms)

	// clear the title field (select all → delete via ctrl+u? easier: type a
	// backspace for each rune). textinput supports KeyBackspace.
	for range "Device-code flow" {
		upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = upd.(Model)
	}
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)

	assert.Equal(t, OverlayEdit, m.overlay, "empty title must stay in the editor")
	assert.Nil(t, cmd, "empty title must not dispatch a store write")
	assert.Equal(t, 0, m.edit.errAt, "error must be attached to the title field")
	ms.AssertNotCalled(t, "UpdateNode", mock.Anything, mock.Anything, mock.Anything)
}

// c on a screen other than detail still opens the create form.
func TestUIEditDoesNotShadowCreateElsewhere(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1042"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = upd.(Model)
	assert.Equal(t, OverlayCreate, m.overlay, "c outside detail must keep creating")
}

// C in detail edits the currently selected step's title/condition, not the
// task itself.
func TestUIEditStepWithC(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1042"
	m.stepCursor = 0 // first step of T-1042 (T-1042.1)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	m = upd.(Model)

	require.Equal(t, OverlayEdit, m.overlay, "C in detail must open the edit overlay")
	require.Equal(t, "T-1042.1", m.edit.nodeID, "C must target the selected step")
}