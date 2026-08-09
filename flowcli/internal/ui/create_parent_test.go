package ui

// The create dialog's searchable parent picker, drawn as its own overlay on
// top of the create form: tasks attach to a work package, steps to a task.
// Typing filters the candidates, enter picks one, and a missing parent blocks
// submission.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// openCreateTask presses o on the WP-AUTH row, which opens a task create with
// the parent row focused (contextual parent preselected).
func openCreateTask(t *testing.T, ms *mockStore) Model {
	t.Helper()
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 0 // WP-AUTH
	m.selectedID = "WP-AUTH"
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	return upd.(Model)
}

// A task create starts on the parent row; the picker lists the project's WPs.
func TestUIParentComboboxListsWorkPackages(t *testing.T) {
	ms := new(mockStore)
	m := openCreateTask(t, ms)

	require.Equal(t, OverlayCreate, m.overlay)
	require.True(t, m.create.parentFocus, "task create must start on the parent row")

	var ids []string
	for _, it := range m.create.parentItems {
		ids = append(ids, it.id)
	}
	require.ElementsMatch(t, []string{"WP-AUTH", "WP-PAY"}, ids,
		"the picker must list the project's work packages")

	view := m.viewCreate(80)
	assert.Contains(t, stripANSI(view), "work package")
}

// Enter opens the picker overlay; typing narrows it; enter picks the
// highlighted WP and returns to the create form.
func TestUIParentComboboxFilterAndPick(t *testing.T) {
	ms := new(mockStore)
	m := openCreateTask(t, ms)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayPickParent, m.overlay, "enter on the parent row opens the picker")

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("auth")})
	m = upd.(Model)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayCreate, m.overlay)
	require.Equal(t, "WP-AUTH", m.create.parentID)
	require.Equal(t, "Authentication", m.create.parentTitle)
	require.True(t, m.create.parentFocus, "focus returns to the parent row after picking")
}

// Esc closes the picker without choosing.
func TestUIParentComboboxEscCloses(t *testing.T) {
	ms := new(mockStore)
	m := openCreateTask(t, ms)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayPickParent, m.overlay)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m = upd.(Model)
	require.Equal(t, OverlayCreate, m.overlay)
	require.Equal(t, "WP-AUTH", m.create.parentID,
		"esc keeps the preselected contextual parent")
	require.True(t, m.create.parentFocus)
}

// The picker is drawn over the create form (both visible in one render).
func TestUIParentPickerOverlaysCreate(t *testing.T) {
	ms := new(mockStore)
	m := openCreateTask(t, ms)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayPickParent, m.overlay)

	full := m.View()
	assert.Contains(t, stripANSI(full), "select work package", "picker title visible")
	assert.Contains(t, stripANSI(full), "new task", "create dialog still visible underneath")
}

// A step create lists the project's tasks (not WPs).
func TestUIParentComboboxStepListsTasks(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // T-1042 task row
	m.selectedID = "T-1042"
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("O")}) // child -> step
	m = upd.(Model)

	require.Equal(t, OverlayCreate, m.overlay)
	require.Equal(t, createStep, m.create.kind)
	require.True(t, m.create.parentFocus)
	assert.Contains(t, stripANSI(m.viewCreate(80)), "task")

	var ids []string
	for _, it := range m.create.parentItems {
		ids = append(ids, it.id)
	}
	require.ElementsMatch(t, []string{"T-1042", "T-2010"}, ids,
		"the picker must list the project's tasks")
}

// Submitting a task without a chosen parent stays in the dialog.
func TestUIParentRequiredOnSubmit(t *testing.T) {
	ms := new(mockStore)
	m := openCreateTask(t, ms)

	// clear the preselected parent, then move to the title field
	m.create.parentID = ""
	m.create.parentTitle = ""

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("New task")})
	m = upd.(Model)
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)

	require.Equal(t, OverlayCreate, m.overlay, "missing parent must keep the dialog open")
	require.True(t, m.create.parentFocus, "focus returns to the parent row")
	require.Nil(t, cmd)
	ms.AssertNotCalled(t, "CreateNode", mock.Anything, mock.Anything)
}

// WP ids in the task-parent picker are padded to a fixed column so the titles
// align, matching the step picker's aligned task list.
func TestUIParentPickerAlignsIDs(t *testing.T) {
	ms := new(mockStore)
	m := openCreateTask(t, ms)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayPickParent, m.overlay)

	// Find the column where each WP title starts; they must agree.
	var starts []int
	for _, line := range strings.Split(stripANSI(m.viewPickParent(100)), "\n") {
		for _, probe := range []string{"Authentication", "Payments"} {
			if i := strings.Index(line, probe); i >= 0 {
				starts = append(starts, wlen(line[:i]))
			}
		}
	}
	require.Len(t, starts, 2, "both WP titles must be visible")
	require.Equal(t, starts[0], starts[1], "titles must start at the same column")
}