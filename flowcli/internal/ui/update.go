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
			m.screen = m.prevScreen
		}
		return m, nil

	case "enter":
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
	}

	switch m.screen {
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
		switch msg.String() {
		case "esc":
			m.overlay = OverlayNone
		case "j", "down":
			m.statusIdx = min(m.statusIdx+1, len(store.AllStatuses)-1)
		case "k", "up":
			m.statusIdx = max(m.statusIdx-1, 0)
		case "enter":
			m.overlay = OverlayNone
			if n, ok := m.current(); ok {
				target := m.ownerTask(n)
				m.lastStatus = &struct {
					id   string
					prev store.Status
				}{target.ID, target.Status}
				return m, m.setStatus(target.ID, store.AllStatuses[m.statusIdx])
			}
		}
		return m, nil

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
			m.finderIdx = min(m.finderIdx+1, max(len(m.finderHits)-1, 0))
			return m, nil
		case "up", "ctrl+p":
			m.finderIdx = max(m.finderIdx-1, 0)
			return m, nil
		case "enter":
			m.overlay = OverlayNone
			m.input.Blur()
			if m.finderIdx < len(m.finderHits) {
				m.selectedID = m.ownerTask(m.finderHits[m.finderIdx]).ID
				m.screen = ScreenDetail
				m.stepCursor = 0
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.finderHits = m.search(m.input.Value())
		m.finderIdx = 0
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
