package ui

// E4 dependency picker: y opens a search for a blocker, cycle candidates are
// dimmed and unselectable, d toggles deps-focus in detail and x removes the
// highlighted edge.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"flowcli/internal/store"
)

// buildDepModel loads a chain T-1 → T-2 → T-3 plus an unrelated T-4, so
// selecting T-1 shows T-2/T-3 as cycle candidates and T-4/WP as addable.
func buildDepModel(t *testing.T, ms *mockStore) Model {
	t.Helper()
	projects := []store.Project{{ID: "prj", Name: "Travel", Description: "booking"}}
	nodes := []store.Node{
		{ID: "WP", ProjectID: "prj", Type: store.WorkPackage, Title: "Auth", Status: store.Ready},
		{ID: "T-1", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Subject", Status: store.Ready},
		{ID: "T-2", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Middle", Status: store.Blocked},
		{ID: "T-3", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Leaf", Status: store.Ready},
		{ID: "T-4", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Unrelated", Status: store.Ready},
	}
	deps := []store.Dependency{
		{BlockerID: "T-1", BlockedID: "T-2"},
		{BlockerID: "T-2", BlockedID: "T-3"},
	}
	ms.On("Projects", mock.Anything).Return(projects, nil).Maybe()
	ms.On("Nodes", mock.Anything, "prj").Return(nodes, nil).Maybe()
	ms.On("Dependencies", mock.Anything, "prj").Return(deps, nil).Maybe()
	ms.On("Activity", mock.Anything, "prj").Return([]store.ActivityEntry{}, nil).Maybe()

	m := New(ms)
	m.projectID = "prj"
	msg := m.load().(loadedMsg)
	upd, _ := m.Update(msg)
	return upd.(Model)
}

// y in detail opens the dependency-add search.
func TestUIDepAddOpens(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	require.Equal(t, OverlayDepAdd, m.overlay)

	// self (T-1) excluded; T-2 and T-3 are cycle candidates; WP and T-4 are
	// addable.
	var addable, cycle []string
	for _, c := range m.depCands {
		if c.cycle {
			cycle = append(cycle, c.node.ID)
		} else {
			addable = append(addable, c.node.ID)
		}
	}
	require.ElementsMatch(t, []string{"WP", "T-4"}, addable)
	require.ElementsMatch(t, []string{"T-2", "T-3"}, cycle)
	// the cursor starts on an addable candidate
	require.False(t, m.depCands[m.depIdx].cycle)
}

// Typing filters the candidates like the finder.
func TestUIDepAddFilters(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("unrel")})
	m = upd.(Model)
	require.Len(t, m.depCands, 1)
	require.Equal(t, "T-4", m.depCands[0].node.ID)
}

// Enter on an addable candidate dispatches AddDependency(blocker, node).
func TestUIDepAddDispatches(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	// land on the unrelated task
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T-4")})
	m = upd.(Model)

	ms.On("AddDependency", mock.Anything, "T-4", "T-1").Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// A cycle candidate is never dispatched, even when enter is pressed on it.
func TestUIDepAddCycleBlocked(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	// filter to a cycle-only set: "T-2" matches only the cycle candidate
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T-2")})
	m = upd.(Model)
	require.Len(t, m.depCands, 1)
	require.True(t, m.depCands[0].cycle)

	// enter must not dispatch; cursor cannot be on the cycle row
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd, "a cycle candidate must never be added")
	ms.AssertNotCalled(t, "AddDependency", mock.Anything, mock.Anything, mock.Anything)
}

// tab toggles deps-focus; backspace on the focused edge asks for
// confirmation, and y removes it.
func TestUIDepRemoveWithDepsFocus(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1" // blocks T-2 (T-1 → T-2)

	// toggle deps focus
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	require.True(t, m.depFocus)

	// backspace on the focused edge opens the confirm dialog, nothing more
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Nil(t, cmd, "backspace must only open the confirm, not remove yet")
	require.Equal(t, OverlayDepRemove, m.overlay)
	require.NotNil(t, m.depRemove)
	require.Equal(t, "T-1", m.depRemove.BlockerID)
	require.Equal(t, "T-2", m.depRemove.BlockedID)

	// confirm with y: the edge is removed, the two nodes stay
	ms.On("RemoveDependency", mock.Anything, "T-1", "T-2").Return(nil).Once()
	upd, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// esc on the remove-confirm cancels and clears the pending edge.
func TestUIDepRemoveEscCancels(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDepRemove, m.overlay)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	require.Nil(t, m.depRemove)
	ms.AssertNotCalled(t, "RemoveDependency", mock.Anything, mock.Anything, mock.Anything)
}

// esc leaves deps-focus before navigating back.
func TestUIDepFocusEscExits(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	require.True(t, m.depFocus)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m = upd.(Model)
	require.False(t, m.depFocus)
	require.Equal(t, ScreenDetail, m.screen, "esc exits deps-focus, not the screen")
}

// d on a node with no deps is a no-op.
func TestUIDepFocusNoDeps(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-4" // no deps

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	require.False(t, m.depFocus, "tab must not focus an empty deps list")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Nil(t, cmd)
	ms.AssertNotCalled(t, "RemoveDependency", mock.Anything, mock.Anything, mock.Anything)
}

// The detail view shows the deps focus cursor on the highlighted row.
func TestUIDepFocusRendersCursor(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"
	m.depFocus = true
	m.width, m.height = 100, 30

	body := stripANSI(m.View())
	assert.Contains(t, body, "▸", "deps-focus must mark the highlighted row")
	assert.Contains(t, body, "deps")
}

// tab cycles back from deps-focus to steps-focus.
func TestUIDepTabCycle(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	require.True(t, m.depFocus)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = upd.(Model)
	require.False(t, m.depFocus, "second tab returns to steps focus")
}

// enter expands/collapses the selected step (tab no longer does).
func TestUIEnterExpandsStep(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1042" // has step T-1042.1

	require.False(t, m.openSteps["T-1042.1"])
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.True(t, m.openSteps["T-1042.1"], "enter must expand the selected step")
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.False(t, m.openSteps["T-1042.1"], "enter again collapses it")
}

// The dependency-add dialog keeps a constant width whether or not a cycle
// candidate is visible (the (cycle) suffix must not widen the box).
func TestUIDepAddConstantWidth(t *testing.T) {
	ms := new(mockStore)
	m := buildDepModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	// filter to a single, non-cycle candidate
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("unrel")})
	m = upd.(Model)
	wNoCycle := wlen(stripANSI(strings.SplitN(m.viewOverlay(100), "\n", 2)[0]))

	// show the full list, which includes cycle rows
	m.depQuery = ""
	m.buildDepCands("T-1")
	wCycle := wlen(stripANSI(strings.SplitN(m.viewOverlay(100), "\n", 2)[0]))

	require.Equal(t, wNoCycle, wCycle, "dialog width must not change when a (cycle) row shows")
}