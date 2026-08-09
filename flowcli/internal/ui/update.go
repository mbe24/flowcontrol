package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		return m, nil

	case loadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.projects, m.nodes, m.deps, m.activity = msg.projects, msg.nodes, msg.deps, msg.activity
		m.index()
		if m.selectedID == "" && len(m.rows) > 0 {
			m.selectedID = m.rows[0].node.ID
		}
		return m, nil

	case refreshedMsg:
		if msg.err == nil {
			m.nodes, m.deps, m.activity = msg.nodes, msg.deps, msg.activity
			m.index()
		}
		return m, nil

	case tea.KeyMsg:
		if m.overlay != OverlayNone {
			return m.updateOverlay(msg)
		}
		return m.updateScreen(msg)
	}
	return m, nil
}

func (m Model) updateScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "1":
		m.screen = ScreenTree
		return m, nil
	case "2":
		m.screen = ScreenLanes
		return m, nil
	case "3":
		m.screen = ScreenChain
		return m, nil

	case "P":
		// back to the project landing screen
		m.screen = ScreenLanding
		return m, nil

	case "o":
		// new sibling below the cursor
		if n, ok := m.current(); ok {
			switch n.Type {
			case store.WorkPackage:
				m.openCreate(createTask, n.ID, "", true)
			case store.Task:
				m.openCreate(createTask, n.ParentID, "", true)
			default:
				m.openCreate(createStep, n.ParentID, "", true)
			}
		}
		return m, nil

	case "O":
		// new child of the cursor
		if n, ok := m.current(); ok {
			switch n.Type {
			case store.WorkPackage:
				m.openCreate(createTask, n.ID, "", true)
			case store.Task:
				m.openCreate(createStep, n.ID, "", true)
			}
		}
		return m, nil

	case "c":
		if m.screen == ScreenDetail {
			// inline edit of title / condition lives in updateDetail
			break
		}
		if m.screen == ScreenChain {
			// creating nodes is not offered on the chain view
			break
		}
		// full create form, kind unlocked. Packages live at project level,
		// so there is no parent to inherit from a cursor row.
		m.openCreate(createPackage, "", "", false)
		return m, nil

	case "?":
		m.overlay = OverlayHelp
		return m, nil

	case "/":
		m.overlay = OverlayFinder
		m.prevScreen = m.screen
		m.input.SetValue("")
		m.input.Placeholder = "task, step or command"
		m.input.Focus()
		m.finderIdx = 0
		m.finderHits = nil
		return m, nil

	case "p":
		m.overlay = OverlayProjects
		m.projectIdx = 0
		return m, nil

	case "esc":
		switch m.screen {
		case ScreenActivity:
			m.screen = ScreenDetail
		case ScreenDetail:
			if m.fromFinder {
				// return to the finder with the same query, selection intact
				m.fromFinder = false
				m.overlay = OverlayFinder
				m.input.Placeholder = "task, step or command"
				m.input.Focus()
				m.screen = m.prevScreen
			} else {
				m.screen = m.prevScreen
			}
		}
		return m, nil

	case "enter":
		if m.screen == ScreenLanding {
			// The landing decides what enter means (open the selected
			// project or the create row). Let it fall through to the
			// per-screen updateLanding handler below instead of opening
			// a node's detail view.
			break
		}
		if n, ok := m.current(); ok {
			m.selectedID = m.ownerTask(n).ID
			m.stepCursor = 0
			m.prevScreen = m.screen
			m.screen = ScreenDetail
		}
		return m, nil

	case "s":
		if _, ok := m.current(); ok {
			m.overlay = OverlayStatus
			m.statusIdx = 0
		}
		return m, nil

	case "u":
		if m.lastStatus != nil {
			id, prev := m.lastStatus.id, m.lastStatus.prev
			m.lastStatus = nil
			m.flash = "undid " + id
			return m, m.setStatus(id, prev)
		}
		return m, nil

	case "backspace", "delete":
		return m.tryDelete()
	}

	switch m.screen {
	case ScreenLanding:
		return m.updateLanding(msg)
	case ScreenTree:
		return m.updateTree(msg)
	case ScreenLanes:
		return m.updateLanes(msg)
	case ScreenChain:
		return m.updateChain(msg)
	case ScreenDetail:
		return m.updateDetail(msg)
	case ScreenActivity:
		return m.updateActivity(msg)
	}
	return m, nil
}

func (m Model) updateTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, len(m.rows)-1)
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "h", "left":
		if len(m.rows) > 0 {
			r := m.rows[m.cursor]
			if r.isWP {
				m.collapsed[r.node.ID] = true
			} else {
				m.collapsed[r.node.ParentID] = true
				for i, rr := range m.rows {
					if rr.node.ID == r.node.ParentID {
						m.cursor = i
					}
				}
			}
			m.buildRows()
		}
	case "l", "right":
		if len(m.rows) > 0 && m.rows[m.cursor].isWP {
			delete(m.collapsed, m.rows[m.cursor].node.ID)
			m.buildRows()
		}
	case "D":
		m.showDone = !m.showDone
		m.buildRows()
	}
	// Keep the cursor visible within the scroll window. The tree reserves
	// `height-4` body rows in frame(); of those, the summary line and the
	// rule take two, and a "completed WPs" line takes one when present.
	win := m.treeVisibleRows()
	if m.cursor < m.treeScroll {
		m.treeScroll = m.cursor
	} else if m.cursor >= m.treeScroll+win {
		m.treeScroll = m.cursor - win + 1
	}
	if len(m.rows) > 0 {
		m.selectedID = m.ownerTask(m.rows[m.cursor].node).ID
	}
	return m, nil
}

func (m Model) updateLanes(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lanes := m.laneSet()
	switch msg.String() {
	case "h", "left":
		m.lane = max(m.lane-1, 0)
	case "l", "right", "tab":
		m.lane = min(m.lane+1, len(lanes)-1)
	case "j", "down":
		n := len(m.laneTasks(lanes[m.lane]))
		m.laneCursor[m.lane] = min(m.laneCursor[m.lane]+1, max(n-1, 0))
	case "k", "up":
		m.laneCursor[m.lane] = max(m.laneCursor[m.lane]-1, 0)
	}
	if n, ok := m.current(); ok {
		m.selectedID = n.ID
	}
	return m, nil
}

func (m Model) updateChain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.chainCursor = min(m.chainCursor+1, len(m.chainRows)-1)
	case "k", "up":
		m.chainCursor = max(m.chainCursor-1, 0)
	case "f":
		if m.focusID != "" {
			m.focusID = ""
		} else if n, ok := m.current(); ok {
			m.focusID = n.ID
		}
		m.chainCursor = 0
		m.buildChain()
	case "w":
		m.chainWP = (m.chainWP + 1) % max(len(m.activeWPs()), 1)
		m.chainCursor = 0
		m.focusID = ""
		m.buildChain()
	}
	if n, ok := m.current(); ok {
		m.selectedID = n.ID
	}
	return m, nil
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	node, ok := m.byID[m.selectedID]
	if !ok {
		return m, nil
	}
	steps := m.stepsOf(node.ID)
	switch msg.String() {
	case "j", "down":
		m.stepCursor = min(m.stepCursor+1, max(len(steps)-1, 0))
	case "k", "up":
		m.stepCursor = max(m.stepCursor-1, 0)
	case "tab":
		if len(steps) > 0 {
			id := steps[m.stepCursor].ID
			m.openSteps[id] = !m.openSteps[id]
		}
	case "v":
		// Accepting over a reported failure is the one thing we confirm.
		b := node.Verification.Badge()
		if !b.Accepted && node.Verification.Agent == store.Fail {
			m.confirmID = node.ID
			m.overlay = OverlayConfirm
			return m, nil
		}
		next := store.Accepted
		if b.Accepted {
			next = store.NoVerdict
		}
		return m, m.setVerdict(node.ID, next)
	case "a":
		m.screen = ScreenActivity
		m.activityScrl = 0
	case "c":
		// inline edit of title / condition (designer: c in detail)
		m.openEdit(node.ID)
	case "C":
		// edit the currently selected step's title / condition
		if len(steps) > 0 {
			m.openEdit(steps[m.stepCursor].ID)
		}
	}
	return m, nil
}

func (m Model) updateActivity(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.activityScrl++
	case "k", "up":
		m.activityScrl = max(m.activityScrl-1, 0)
	case "i":
		m.overlay = OverlayComment
		m.input.SetValue("")
		m.input.Placeholder = "leave a note…"
		m.input.Focus()
	}
	return m, nil
}

func (m Model) updateOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case OverlayCreate:
		return m.updateCreate(msg)
	case OverlayEdit:
		return m.updateEdit(msg)
	case OverlayCascade:
		return m.updateCascade(msg)
	case OverlayDelete:
		switch msg.String() {
		case "esc", "n", "q":
			m.overlay, m.deleteInfo, m.deleteScroll = OverlayNone, nil, 0
		case "y", "enter":
			id := m.deleteInfo.id
			m.overlay, m.deleteInfo, m.deleteScroll = OverlayNone, nil, 0
			m.flash = "deleted " + id + " · u to undo"
			return m, m.doDelete(id)
		case "j", "down":
			// scroll by line; the clamp mirrors the view's own total
			var total int
			for _, e := range m.deleteInfo.unblocked {
				total += len(effectLines(46, e))
			}
			m.deleteScroll = min(m.deleteScroll+1, max(0, total-deleteLines))
		case "k", "up":
			m.deleteScroll = max(m.deleteScroll-1, 0)
		}
		return m, nil
	case OverlayHelp:
		switch msg.String() {
		case "esc", "?", "q":
			m.overlay = OverlayNone
		}
		return m, nil

	case OverlayConfirm:
		switch msg.String() {
		case "y":
			id := m.confirmID
			m.overlay, m.confirmID = OverlayNone, ""
			m.flash = id + " · accepted over the agent's failure"
			return m, m.setVerdict(id, store.Accepted)
		case "esc", "n", "q":
			m.overlay, m.confirmID = OverlayNone, ""
		}
		return m, nil

	case OverlayStatus:
		// chooseStatus is the shared path for Enter and the r/b/x/d shortcuts:
		// plan the cascade first (a no-effect change applies immediately), else
		// open the cascade preview.
		chooseStatus := func(to store.Status) (tea.Model, tea.Cmd) {
			if n, ok := m.current(); ok {
				target := m.ownerTask(n)
				cc, plan := m.planCascade(target, to)
				if !plan {
					m.overlay = OverlayNone
					m.lastStatus = &struct {
						id   string
						prev store.Status
					}{target.ID, target.Status}
					m.flash = target.ID + " → " + string(to) + " · nothing was waiting"
					return m, m.setStatus(target.ID, to)
				}
				m.overlay = OverlayCascade
				m.cascade = cc
			}
			return m, nil
		}
		switch msg.String() {
		case "esc":
			m.overlay = OverlayNone
		case "j", "down":
			m.statusIdx = min(m.statusIdx+1, len(store.AllStatuses)-1)
		case "k", "up":
			m.statusIdx = max(m.statusIdx-1, 0)
		case "enter":
			return chooseStatus(store.AllStatuses[m.statusIdx])
		case "r":
			return chooseStatus(store.Ready)
		case "b":
			return chooseStatus(store.Blocked)
		case "x":
			return chooseStatus(store.Deferred)
		case "d":
			return chooseStatus(store.Done)
		}

	case OverlayProjects:
		switch msg.String() {
		case "esc":
			m.overlay = OverlayNone
		case "j", "down":
			m.projectIdx = min(m.projectIdx+1, len(m.projects)-1)
		case "k", "up":
			m.projectIdx = max(m.projectIdx-1, 0)
		case "enter":
			m.overlay = OverlayNone
			if m.projectIdx < len(m.projects) {
				m.projectID = m.projects[m.projectIdx].ID
				m.cursor, m.chainCursor, m.selectedID = 0, 0, ""
				m.screen = ScreenTree
				return m, m.load
			}
		}
		return m, nil

	case OverlayComment:
		switch msg.String() {
		case "esc":
			m.overlay = OverlayNone
			m.input.Blur()
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			m.overlay = OverlayNone
			m.input.Blur()
			if text != "" && m.selectedID != "" {
				return m, m.addComment(m.selectedID, text)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case OverlayFinder:
		switch msg.String() {
		case "esc":
			m.overlay = OverlayNone
			m.input.Blur()
			return m, nil
		case "down", "ctrl+n":
			n := max(len(m.finderHits)-1, 0)
			if m.finderIdx < n {
				m.finderIdx++
			}
			// keep selection within the visible window
			if m.finderIdx >= m.finderScroll+finderVisible {
				m.finderScroll = m.finderIdx - finderVisible + 1
			}
			return m, nil
		case "up", "ctrl+p":
			if m.finderIdx > 0 {
				m.finderIdx--
			}
			if m.finderIdx < m.finderScroll {
				m.finderScroll = m.finderIdx
			}
			return m, nil
		case "enter":
			m.overlay = OverlayNone
			m.input.Blur()
			if m.finderIdx < len(m.finderHits) {
				m.selectedID = m.ownerTask(m.finderHits[m.finderIdx]).ID
				m.screen = ScreenDetail
				m.stepCursor = 0
				// remember the finder so ESC in the detail returns to it with
				// the same query intact
				m.fromFinder = true
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.finderHits = m.search(m.input.Value())
		m.finderIdx = 0
		m.finderScroll = 0
		return m, cmd
	}
	return m, nil
}

// ownerTask maps a step to the task that owns it; anything else is itself.
func (m *Model) ownerTask(n store.Node) store.Node {
	if n.Type == store.Step {
		if parent, ok := m.byID[n.ParentID]; ok {
			return parent
		}
	}
	return n
}

func (m *Model) search(q string) []store.Node {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var out []store.Node
	for _, n := range m.nodes {
		if n.Type == store.WorkPackage {
			continue
		}
		if strings.Contains(strings.ToLower(n.Title), q) || strings.Contains(strings.ToLower(n.ID), q) {
			out = append(out, n)
		}
		if len(out) >= 10 {
			break
		}
	}
	return out
}

func (m Model) setStatus(id string, s store.Status) tea.Cmd {
	return func() tea.Msg {
		_ = m.store.SetStatus(m.ctx, id, s)
		return m.refresh()
	}
}

// tryDelete handles backspace/delete. A leaf with no edges deletes straight
// away (status line offers u); anything with collateral opens the confirm
// dialog so the cascade is named before it happens.
func (m Model) tryDelete() (tea.Model, tea.Cmd) {
	n, ok := m.current()
	if !ok {
		return m, nil
	}
	// Deleting a step is done from its owning task's view; the owner is the
	// row the cursor is really on for steps, but deletion targets the node
	// itself (a step's children are meaningless). Use the raw node.
	target := n
	col := m.deleteCollateral(target.ID)
	if col.taskCount == 0 && col.stepCount == 0 && col.edgeCount == 0 {
		// leaf with no edges: just do it, undo is one u away
		m.flash = "deleted " + target.ID + " · u to undo"
		return m, m.doDelete(target.ID)
	}
	m.deleteInfo = col
	m.overlay = OverlayDelete
	return m, nil
}

// deleteCollateral computes what deleting id would remove/keep/unblock.
func (m *Model) deleteCollateral(id string) *deleteCollateral {
	sub := m.subtree(id)
	col := &deleteCollateral{id: id, title: m.byID[id].Title}
	// the work package that owns the node being deleted; unblocked dependents
	// from the same package get no WP line in the unblocks section (only
	// cross-package ones do).
	deadWP := m.byID[id].ParentID

	// descendants (everyone in the subtree except the root), split by kind
	for _, n := range m.nodes {
		if n.ID == id {
			continue
		}
		if !sub[n.ID] {
			continue
		}
		switch n.Type {
		case store.Task:
			col.taskCount++
		case store.Step:
			col.stepCount++
		}
	}

	// edges where either endpoint is in the subtree
	for _, d := range m.deps {
		if sub[d.BlockerID] || sub[d.BlockedID] {
			col.edgeCount++
		}
	}

	// What unblocks: a dependent OUTSIDE the subtree whose blockers all vanish
	// with the deletion — i.e. every blocker of the dependent is inside the
	// subtree. Rendered like the cascade preview.
	for _, d := range m.deps {
		if !sub[d.BlockedID] && sub[d.BlockerID] {
			dep, ok := m.byID[d.BlockedID]
			if !ok {
				continue
			}
			// A dependent is still stuck if it has a blocker outside the subtree.
			stuck := false
			for _, b := range m.blockers[dep.ID] {
				if !sub[b] {
					if other, ok := m.byID[b]; ok && other.Status != store.Done {
						stuck = true
					}
				}
			}
			e := effect{node: dep, from: dep.Status, to: store.Ready}
			if stuck {
				e.stuck = true
				e.to = dep.Status
			}
			if p, ok := m.byID[dep.ParentID]; ok && dep.ParentID != deadWP {
				e.crossWP = p.Title
			}
			col.unblocked = append(col.unblocked, e)
		}
	}

	// activity entries for the subtree are kept (they're history)
	for _, a := range m.activity {
		if sub[a.NodeID] {
			col.actCount++
		}
	}
	return col
}

// subtree returns the map of ids reachable from root (root + descendants).
func (m *Model) subtree(root string) map[string]bool {
	out := map[string]bool{root: true}
	changed := true
	for changed {
		changed = false
		for _, n := range m.nodes {
			if out[n.ParentID] && !out[n.ID] {
				out[n.ID] = true
				changed = true
			}
		}
	}
	return out
}

func (m Model) doDelete(id string) tea.Cmd {
	return func() tea.Msg {
		_ = m.store.DeleteNode(m.ctx, id)
		return m.refresh()
	}
}

func (m Model) setVerdict(id string, v store.HumanVerdict) tea.Cmd {
	return func() tea.Msg {
		_ = m.store.SetVerdict(m.ctx, id, v)
		return m.refresh()
	}
}

func (m Model) addComment(id, text string) tea.Cmd {
	return func() tea.Msg {
		_ = m.store.AddComment(m.ctx, id, text)
		return m.refresh()
	}
}
