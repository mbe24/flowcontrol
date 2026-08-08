package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"flowcli/internal/store"
)

// A leaf with no edges deletes straight away — backspace skips the confirm
// (designer rule: "⌫ deletes, status line offers u").
func TestUIDeleteLeafDirect(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenDetail // current() resolves via selectedID here
	m.selectedID = "T-1042.1"

	ms.On("DeleteNode", mock.Anything, "T-1042.1").Return(nil).Once()

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay, "leaf delete must not open a confirm")
	require.Contains(t, m.flash, "deleted T-1042.1")
	execCmd(t, cmd)
	ms.AssertExpectations(t)
}

// A node with descendants opens the confirm dialog naming the collateral.
func TestUIDeleteWithCollateralOpensConfirm(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // T-1042, which has a step child
	m.selectedID = "T-1042"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay, "expected delete confirm for a node with descendants")
	require.NotNil(t, m.deleteInfo)
	require.Equal(t, 1, m.deleteInfo.stepCount, "T-1042 has one step descendant")
	// the confirm names the collateral so the dialog earns the cascade
	view := stripANSI(m.viewOverlay(80))
	require.Contains(t, view, "step nodes", "a task's descendants are its steps")
	require.NotContains(t, view, "%d", "count must be substituted, not a verbatim %d")
	// a divider row separates the content from the action keys even
	// when there is no unblocks section (boxWithKeys draws the divider)
	lines := strings.Split(view, "\n")
	for i := range lines {
		if strings.Contains(lines[i], "activity entries") {
			require.Contains(t, lines[i+1], "├", "divider row separates content from keys")
			require.Contains(t, lines[i+2], "[esc] cancel", "keys follow the divider")
			break
		}
	}
	// no store call yet
	ms.AssertNotCalled(t, "DeleteNode", mock.Anything, mock.Anything)
}

// Confirm (y) dispatches the DeleteNode write.
func TestUIDeleteConfirmDispatchesStore(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1042"
	ms.On("DeleteNode", mock.Anything, "T-1042").Return(nil).Once()

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay, "backspace on collateral must open the confirm")
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay, "confirm must close the overlay")
	require.Contains(t, m.flash, "deleted T-1042")
	execCmd(t, cmd)
	ms.AssertCalled(t, "DeleteNode", mock.Anything, "T-1042")
}

// ESC cancels: no write, dialog gone.
func TestUIDeleteEscCancels(t *testing.T) {
	ms := new(mockStore)
	m := loadMockedModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1042"

	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = upd.(Model)
	require.Equal(t, OverlayNone, m.overlay)
	require.Nil(t, m.deleteInfo)
	ms.AssertNotCalled(t, "DeleteNode", mock.Anything, mock.Anything)
}

// Deleting a node that other tasks depend on lists those dependents as
// unblocked, rendered with the same "unblocks" layout as the mark-done dialog.
func TestUIDeleteListsUnblocked(t *testing.T) {
	ms := new(mockStore)
	m := buildDeleterModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // subject
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay)
	require.Len(t, m.deleteInfo.unblocked, 1, "one dependent becomes ready")
	view := stripANSI(m.viewOverlay(100))
	require.Contains(t, view, "unblocks 1")
	require.Contains(t, view, "Dependent task")
	require.Contains(t, view, "BLOCKED → READY", "the dependent's verdict uses cascade styling")
}

// A dependent that stays blocked by someone else shows as "still blocked",
// exactly like the mark-done dialog lists stuck effects.
func TestUIDeleteListsStuckDependent(t *testing.T) {
	ms := new(mockStore)
	m := buildDeleterModel(t, ms)
	// T-2 is blocked by T-1 (deleted) AND T-3 (kept): stays blocked.
	m.deps = append(m.deps, store.Dependency{BlockerID: "T-3", BlockedID: "T-2"})
	m.index()
	m.screen = ScreenTree
	m.cursor = 1
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay)
	view := stripANSI(m.viewOverlay(100))
	require.Contains(t, view, "still blocked", "a dependent blocked by someone else stays stuck")
}

// buildDeleterModel loads a fixture where deleting T-1 unblocks T-2.
func buildDeleterModel(t *testing.T, ms *mockStore) Model {
	t.Helper()
	projects := []store.Project{{ID: "prj", Name: "Travel", Description: "booking"}}
	nodes := []store.Node{
		{ID: "WP", ProjectID: "prj", Type: store.WorkPackage, Title: "Auth", Status: store.Ready},
		{ID: "T-1", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Subject", Status: store.Ready},
		{ID: "T-2", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Dependent task", Status: store.Blocked},
		{ID: "T-3", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Other blocker", Status: store.Ready},
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
	return upd.(Model)
}

// buildDeleterCrossWPModel loads a fixture where deleting T-1 unblocks
// T-2, and T-2 lives in a DIFFERENT work package from T-1, so the unblocks
// section shows a cross-package line for the dependent.
func buildDeleterCrossWPModel(t *testing.T, ms *mockStore) Model {
	t.Helper()
	projects := []store.Project{{ID: "prj", Name: "Travel", Description: "booking"}}
	nodes := []store.Node{
		{ID: "WP", ProjectID: "prj", Type: store.WorkPackage, Title: "Auth", Status: store.Ready},
		{ID: "WP2", ProjectID: "prj", Type: store.WorkPackage, Title: "Payments", Status: store.Ready},
		{ID: "T-1", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Subject", Status: store.Ready},
		{ID: "T-2", ProjectID: "prj", ParentID: "WP2", Type: store.Task, Title: "Dependent task", Status: store.Blocked},
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
	return upd.(Model)
}

// A dependents work package line only appears when it differs from the
// deleted node's package: same-package dependents get no WP line, cross-
// package ones do.
func TestUIDeleteUnblocksWPCrossOnly(t *testing.T) {
	// cross-package dependent → its WP name IS listed
	ms := new(mockStore)
	m := buildDeleterCrossWPModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // T-1
	m.selectedID = "T-1"
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay)
	view := stripANSI(m.viewOverlay(80))
	require.Contains(t, view, "Payments", "cross-package dependent lists its WP name")

	// same-package dependent → no WP line (buildDeleterModel: T-2 shares WP)
	ms2 := new(mockStore)
	m2 := buildDeleterModel(t, ms2)
	m2.screen = ScreenTree
	m2.cursor = 1
	m2.selectedID = "T-1"
	upd2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m2 = upd2.(Model)
	view2 := stripANSI(m2.viewOverlay(80))
	for _, l := range strings.Split(view2, "\n") {
		require.NotContains(t, l, "Auth", "same-package dependent shows no WP line")
	}
}
// lanes) without a scrollbar.
func TestUIDeleteScrollsUnblocked(t *testing.T) {
	ms := new(mockStore)
	m := buildManyDependentsModel(t, ms)
	m.screen = ScreenTree
	m.cursor = 1 // subject
	m.selectedID = "T-1"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay)

	// first window shows the first deleteLines lines, not the whole list
	view := stripANSI(m.viewOverlay(100))
	require.Contains(t, view, "unblocks 6")
	require.Contains(t, view, "Dep 1")
	require.NotContains(t, view, "Dep 6", "last dependent starts off the window")

	// j scrolls forward one line (not one whole dependent)
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = upd.(Model)
	require.Equal(t, 1, m.deleteScroll, "j scrolls by exactly one line")

	// scrolling past the first dependent's ID line drops the "Dep 1" ID line
	// from the top; after enough j presses the window reaches the tail
	for range 20 {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = upd.(Model)
	}
	view = stripANSI(m.viewOverlay(100))
	require.Contains(t, view, "Dep 6", "last dependent is reachable by scrolling")

	// the dialog keeps a constant height while scrolling: every window shows
	// exactly deleteLines content lines, so trailing windows are blank-padded
	// instead of shrinking the dialog
	heightAtTail := strings.Count(view, "\n")

	// k scrolls back toward the top
	for range 30 {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = upd.(Model)
	}
	require.Equal(t, 0, m.deleteScroll, "k returns to the top")
	view = stripANSI(m.viewOverlay(100))
	require.Contains(t, view, "Dep 1")
	require.Equal(t, strings.Count(view, "\n"), heightAtTail,
		"dialog height at the top matches the tail window")

	// and at every scroll position in between
	for range 10 {
		upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = upd.(Model)
		view = stripANSI(m.viewOverlay(100))
		require.Equal(t, strings.Count(view, "\n"), heightAtTail,
			"dialog height is constant at scroll %d", m.deleteScroll)
	}
}

// buildManyDependentsModel: deleting T-1 unblocks six dependents (every dep
// points at a distinct blocked task), so the list exceeds deleteLines.
func buildManyDependentsModel(t *testing.T, ms *mockStore) Model {
	t.Helper()
	projects := []store.Project{{ID: "prj", Name: "Travel", Description: "booking"}}
	nodes := []store.Node{
		{ID: "WP", ProjectID: "prj", Type: store.WorkPackage, Title: "Auth", Status: store.Ready},
		{ID: "T-1", ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: "Subject", Status: store.Ready},
	}
	var deps []store.Dependency
	// dependents Dep 1..6, each blocked by T-1
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("T-%d", 10+i)
		nodes = append(nodes, store.Node{ID: id, ProjectID: "prj", ParentID: "WP", Type: store.Task, Title: fmt.Sprintf("Dep %d", i), Status: store.Blocked})
		deps = append(deps, store.Dependency{BlockerID: "T-1", BlockedID: id})
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

// Deleting a work package names its task and step descendants on separate
// lines (always both), while a task names only its steps.
func TestUIDeleteWorkPackageShowsTasksAndSteps(t *testing.T) {
	ms := new(mockStore)
	m := buildDeleterModel(t, ms) // WP has three task children, no steps
	m.screen = ScreenTree
	m.cursor = 0
	m.selectedID = "WP"

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay)
	require.Equal(t, 3, m.deleteInfo.taskCount)
	require.Equal(t, 0, m.deleteInfo.stepCount)

	view := stripANSI(m.viewOverlay(80))
	viewLines := strings.Split(view, "\n")
	taskLine, stepLine := 0, 0
	// The label's first word must start at a fixed column (the end of the
	// fixed-width number column), regardless of the count's digit width.
	firstWordCol := func(l string, firstWord string) int { return strings.Index(l, firstWord) }
	taskCol, stepCol := -1, -1
	for i, l := range viewLines {
		if strings.Contains(l, "task nodes") {
			taskLine = i
			taskCol = firstWordCol(l, "task")
		}
		if strings.Contains(l, "step nodes") {
			stepLine = i
			stepCol = firstWordCol(l, "step")
		}
	}
	require.True(t, taskLine > 0 && stepLine > 0, "both label lines are rendered")
	require.Equal(t, taskLine+1, stepLine, "step line must directly follow the task line")
	// numbers are left-aligned in a fixed-width column, so the labels line up
	require.Equal(t, taskCol, stepCol, "labels start at the same column regardless of digit count")

	// all collateral labels (nodes + edges + entries) share the same start
	// column because the count column is fixed width
	labelStarts := map[string]int{}
	for _, l := range viewLines {
		for _, want := range []string{"task nodes", "dependency edges", "activity entries"} {
			first := strings.Fields(want)[0]
			if strings.Contains(l, want) {
				if _, ok := labelStarts[want]; !ok {
					labelStarts[want] = strings.Index(l, first)
				}
			}
		}
	}
	require.Equal(t, taskCol, labelStarts["task nodes"], "edges label aligns with node labels")
	require.Equal(t, taskCol, labelStarts["dependency edges"], "edges label aligns with node labels")
	require.Equal(t, taskCol, labelStarts["activity entries"], "entries label aligns with node labels")

	// Right-aligned tags: "deleted" (red) next to each count, "removed" and
	// "kept" for the activity line. All tags
	// share the same right edge (right-aligned column), like the status dialog.
	rightOf := func(ln int, word string) int {
		return strings.Index(viewLines[ln], word) + len(word)
	}
	// deleted and removed are both 7 chars → they must start at the same column.
	deletedTask := strings.Index(viewLines[taskLine], "deleted")
	removed := strings.Index(viewLines[taskLine+2], "removed")
	require.True(t, deletedTask > 0, "deleted tag exists")
	require.Equal(t, deletedTask, removed, "deleted and removed align at the same column")
	// the activity tag is longer so it starts further left, but ends at the
	// same right edge as the others.
	require.True(t, rightOf(taskLine, "deleted") == rightOf(taskLine+2, "removed"),
		"deleted and removed end at the same right edge")
	require.Contains(t, view, "kept", "activity line carries the kept tag")
}

// A long unblocked-dependent title wraps onto indented continuation lines
// instead of being cut off at the dialog edge.
func TestUIDeleteUnblocksTitleWraps(t *testing.T) {
	ms := new(mockStore)
	m := buildDeleterCrossWPModel(t, ms)
	long := "Set up the authentication infrastructure for the travel booking web application with OIDC and session management"
	n := m.byID["T-2"]
	n.Title = long
	m.byID["T-2"] = n
	m.screen = ScreenTree
	m.cursor = 1 // T-1 -> unblocks T-2
	m.selectedID = "T-1"
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = upd.(Model)
	require.Equal(t, OverlayDelete, m.overlay)

	view := stripANSI(m.viewOverlay(80))
	// every word of the title survives (no truncation)
	for _, word := range strings.Fields(long) {
		require.Contains(t, view, word, "wrapped title keeps word %q", word)
	}
	// title lines wrap indented at the same 4-space column. Compare display
	// columns (wlen), not byte offsets: the status glyph is multi-byte so
	// strings.Index would misreport the offset.
	lines := strings.Split(view, "\n")
	idLine := 0
	for i, l := range lines {
		if strings.Contains(l, "● T-2") {
			idLine = i
		}
	}
	require.True(t, idLine > 0, "ID line rendered")
	// every line from the first wrapped title line up to (but excluding) the
	// WP/status lines starts with the same 4-space indent
	var wrapped []string
	for i := idLine + 1; i < len(lines); i++ {
		l := lines[i]
		if l == "" || strings.Contains(l, "BLOCKED") || strings.Contains(l, "READY") ||
			strings.TrimSpace(l) == "Auth" {
			break
		}
		wrapped = append(wrapped, l)
	}
	require.GreaterOrEqual(t, len(wrapped), 2, "title wraps onto multiple lines")
	base := -1
	for _, l := range wrapped {
		col := wlen(strings.SplitN(l, strings.Fields(l)[0], 2)[0])
		if base < 0 {
			base = col
		}
		require.Equal(t, base, col, "wrapped title line aligns at col %d", base)
	}
	wpHasBrackets := false
	for _, l := range lines {
		if strings.TrimSpace(strings.ReplaceAll(l, "│", "")) == "Auth" {
			wpHasBrackets = false // matches plain WP name line
		}
		if strings.Contains(l, "⟨Auth⟩") {
			wpHasBrackets = true
		}
	}
	require.False(t, wpHasBrackets, "WP name rendered without angle brackets")
	require.NotContains(t, view, "⟨Auth⟩", "WP name has no angle brackets")
}
