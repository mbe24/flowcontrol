package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"flowcli/internal/store"
)

// --- fixtures -----------------------------------------------------------------

func fixtureContent() ([]store.Project, []store.Node, []store.Dependency, []store.ActivityEntry) {
	projects := []store.Project{{ID: "prj-travel", Name: "Travel Webapp", Description: "Booking flow."}}
	nodes := []store.Node{
		{ID: "WP-AUTH", ProjectID: "prj-travel", Type: store.WorkPackage, Title: "Authentication", Status: store.Ready},
		{ID: "WP-PAY", ProjectID: "prj-travel", Type: store.WorkPackage, Title: "Payments", Status: store.Ready},
		{ID: "T-1042", ProjectID: "prj-travel", ParentID: "WP-AUTH", Type: store.Task, Title: "Device-code flow", Status: store.Ready,
			Condition: "pnpm test:auth --grep device", Verification: store.Verification{Agent: store.Pass}},
		{ID: "T-1042.1", ProjectID: "prj-travel", ParentID: "T-1042", Type: store.Step, Title: "Register client", Status: store.Done},
		{ID: "T-2010", ProjectID: "prj-travel", ParentID: "WP-PAY", Type: store.Task, Title: "Checkout session", Status: store.Ready},
	}
	deps := []store.Dependency{}
	activity := []store.ActivityEntry{}
	return projects, nodes, deps, activity
}

// loadMockedModel builds a model backed by the test's mock store and feeds it a
// loadedMsg so nodes/deps/activity are indexed, exactly like a real first load.
// Reads are mocked to return the fixture.
func loadMockedModel(t *testing.T, ms *mockStore) Model {
	t.Helper()
	projects, nodes, deps, activity := fixtureContent()
	ms.On("Projects", mock.Anything).Return(projects, nil).Maybe()
	ms.On("Nodes", mock.Anything, "prj-travel").Return(nodes, nil).Maybe()
	ms.On("Dependencies", mock.Anything, "prj-travel").Return(deps, nil).Maybe()
	ms.On("Activity", mock.Anything, "prj-travel").Return(activity, nil).Maybe()

	m := New(ms)
	msg := m.load().(loadedMsg)
	upd, _ := m.Update(msg)
	return upd.(Model)
}

// execCmd runs a tea.Cmd (the async write) and returns the resulting tea.Msg,
// mirroring how Bubble Tea's runtime consumes a returned command.
func execCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd, "expected a non-nil tea.Cmd; the UI action should dispatch a store write")
	return cmd()
}

// --- status -------------------------------------------------------------------

// The status overlay (s) → enter applies SetStatus to the owner task of the
// selected node. In the tree, T-1042 is under the cursor, so SetStatus must be
// called with the chosen status.
func TestUIStatusTriggersStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	// select T-1042
	m.cursor = 1
	m.selectedID = "T-1042"

	// press s to open the status overlay
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	require.Equal(t, OverlayStatus, m.overlay, "expected status overlay after s")

	// AllStatuses = [Ready, Blocked, Deferred, Done]; the cursor starts at
	// index 0 (Ready). Move to index 3 (Done).
	for i := 0; i < 3; i++ {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	require.Equal(t, 3, m.statusIdx, "status cursor should be on Done")

	// expect the store call with Done on the owner task
	ms.On("SetStatus", mock.Anything, "T-1042", store.Done).Return(nil).Once()

	// press enter → executes SetStatus and returns a refresh cmd
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay, "status overlay should close on enter")
	execCmd(t, cmd)

	ms.AssertExpectations(t)
}

// Status on a step applies to its owner task, not the step itself.
func TestUIStatusStepTriggersOwnerTask(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	// In detail view current() returns byID[selectedID]; selecting the step and
	// setting its status must target the owning task T-1042.
	m.screen = ScreenDetail
	m.selectedID = "T-1042.1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	for i := 0; i < 1; i++ { // index 1 = Blocked
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	ms.On("SetStatus", mock.Anything, "T-1042", store.Blocked).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// Undo (u) resends the previous status to the same owner task.
func TestUIRedoUndoTriggersStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1042"

	// set T-1042 → Done
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	for i := 0; i < 3; i++ {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = upd.(Model)
	}
	ms.On("SetStatus", mock.Anything, "T-1042", store.Done).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	execCmd(t, cmd)

	// undo: SetStatus back to the previous status (Ready)
	ms.On("SetStatus", mock.Anything, "T-1042", store.Ready).Return(nil).Once()
	upd, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = upd.(Model)
	execCmd(t, cmd)

	ms.AssertExpectations(t)
}

// --- verdict ------------------------------------------------------------------

// v on a non-failed, non-accepted task records an Accepted verdict.
func TestUIVerifyTriggersStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenDetail
	m.selectedID = "T-1042"

	// T-1042 has Agent=Pass (Accepted badge false) → v should SetVerdict Accepted
	ms.On("SetVerdict", mock.Anything, "T-1042", store.Accepted).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// v on an already-accepted task clears the verdict (NoVerdict).
func TestUIVerifyClearsWhenAccepted(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenDetail
	// make T-1042 human-accepted
	upd, _ := m.Update(loadedMsg{nodes: []store.Node{
		{ID: "T-1042", ProjectID: "prj-travel", Type: store.Task, Title: "Device-code", Status: store.Ready,
			Condition: "cond", Verification: store.Verification{Agent: store.Pass, Human: store.Accepted}},
	}, deps: []store.Dependency{}, activity: []store.ActivityEntry{}})
	m = upd.(Model)
	m.screen = ScreenDetail
	m.selectedID = "T-1042"

	ms.On("SetVerdict", mock.Anything, "T-1042", store.NoVerdict).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// --- comment ------------------------------------------------------------------

// Activity view (a) → i opens the comment overlay; typing text + enter calls
// AddComment with the note text on the selected node.
func TestUICommentTriggersStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenActivity
	m.selectedID = "T-1042"

	// open comment overlay
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = upd.(Model)
	require.Equal(t, OverlayComment, m.overlay, "expected comment overlay after i")

	// type text
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("remember to rotate")})
	m = upd.(Model)

	ms.On("AddComment", mock.Anything, "T-1042", "remember to rotate").Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// addComment must not fire on whitespace-only input.
func TestUICommentEmptyDoesNotTriggerStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenActivity
	m.selectedID = "T-1042"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = upd.(Model)
	// type only spaces
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("   ")})
	m = upd.(Model)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// empty note → no store call, no cmd
	assert.Nil(t, cmd, "whitespace comment must not dispatch AddComment")
	ms.AssertNotCalled(t, "AddComment", mock.Anything, mock.Anything, mock.Anything)
}

// --- refresh on write -----------------------------------------------------------

// The store write must be followed by a refresh so the UI reflects the change.
func TestUIStatusReturnsRefreshCmd(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1042"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = upd.(Model)
	ms.On("SetStatus", mock.Anything, "T-1042", store.Ready).Return(nil).Once()
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(Model)

	// the returned cmd runs SetStatus then issues a refresh (a tea.Msg)
	msg := execCmd(t, cmd)
	_, ok := msg.(refreshedMsg)
	assert.True(t, ok, "after a status write the UI should refresh (got %T)", msg)
	ms.AssertExpectations(t)
}
