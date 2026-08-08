package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"flowcli/internal/store"
)

// buildCascadeModel returns a model loaded with a fixture that exercises the
// cascade path: a task with an open (non-DONE) step, plus a blocked dependent
// task that becomes ready when the subject is marked done.
func buildCascadeModel(t *testing.T, ms *mockStore) Model {
	t.Helper()
	projects := []store.Project{{ID: "prj", Name: "Travel", Description: "booking"}}
	nodes := []store.Node{
		{ID: "WP", ProjectID: "prj", Type: store.WorkPackage, Title: "Auth", Status: store.Ready},
		{ID: "T-1", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Subject task", Status: store.Ready},
		{ID: "T-1.1", ProjectID: "prj", ParentID: "T-1", Type: store.Step, Title: "open step", Status: store.Ready},
		{ID: "T-2", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Dependent task", Status: store.Blocked},
	}
	deps := []store.Dependency{{BlockerID: "T-1", BlockedID: "T-2"}}
	ms.On("Projects", mock.Anything).Return(projects, nil).Maybe()
	ms.On("Nodes", mock.Anything, "prj").Return(nodes, nil).Maybe()
	ms.On("Dependencies", mock.Anything, "prj").Return(deps, nil).Maybe()
	ms.On("Activity", mock.Anything, "prj").Return([]store.ActivityEntry{}, nil).Maybe()

	m := New(ms)
	m.projectID = "prj"
	msg := m.load().(loadedMsg)
	upd, _ := m.Update(msg)
	m = upd.(Model)
	m.screen = ScreenTree
	m.cursor = 1 // T-1
	m.selectedID = "T-1"
	return m
}

// Marking a task done with an open step and a blocked dependent plans a
// cascade and opens the cascade overlay instead of applying the write
// immediately.
func TestUIStatusPlansCascade(t *testing.T) {
	ms := new(mockStore)
	m := buildCascadeModel(t, ms)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	require.Equal(t, OverlayStatus, m.overlay)
	// move cursor to Done (index 3)
	for i := 0; i < 3; i++ {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayCascade, m.overlay, "expected cascade overlay for a task with open steps/dependents")
	require.True(t, m.cascade.active)
	require.Equal(t, 1, m.cascade.openSteps, "one step should be open")
	require.Len(t, m.cascade.effects, 1, "one blocked dependent should be listed")
}

// No cascade (no open steps, no dependents) applies immediately and flashes
// "nothing was waiting", exactly as before the rewrite.
func TestUIStatusNoCascadeAppliesImmediately(t *testing.T) {
	ms := new(mockStore)
	// Reuse the standard fixtureContent model: T-1042 -> Done with no deps and
	// only DONE steps, so nothing cascades.
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1042"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	for i := 0; i < 3; i++ { // move to Done
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	ms.On("SetStatus", mock.Anything, "T-1042", store.Done).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	require.Contains(t, m.flash, "nothing was waiting")
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// Enter on the cascade: with open steps, the first enter asks the close-steps
// question (stage 1); y applies and writes the subject + the open step.
func TestUICascadeApplyClosesOpenSteps(t *testing.T) {
	ms := new(mockStore)
	m := buildCascadeModel(t, ms)

	// open the status overlay and choose Done
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	for i := 0; i < 3; i++ {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayCascade, m.overlay)
	require.Equal(t, 0, m.cascade.stage, "stage 0 = review the cascade")

	// first enter with open steps -> stage 1 (close-steps question)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, 1, m.cascade.stage, "should ask about open steps")
	require.NotEmpty(t, m.View(), "stage-1 view should render")

	// y = close all -> write subject T-1 -> Done and open step T-1.1 -> Done
	ms.On("SetStatus", mock.Anything, "T-1.1", store.Done).Return(nil).Once()
	ms.On("SetStatus", mock.Anything, "T-1", store.Done).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// ESC on the cascade cancels and returns to the previous state (no writes).
func TestUICascadeEscCancels(t *testing.T) {
	ms := new(mockStore)
	m := buildCascadeModel(t, ms)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	for i := 0; i < 3; i++ {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayCascade, m.overlay)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	require.False(t, m.cascade.active)
	ms.AssertNotCalled(t, "SetStatus", mock.Anything, mock.Anything, mock.Anything)
}
