package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"flowcontrol/fctrl/internal/store"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case projectsMsg:
		m.projects = msg.ps
		if m.projectID == "" && len(m.projects) > 0 {
			// stay on the picker; the user chooses
			m.projCursor = 0
		}
		return m, nil

	case dataMsg:
		m.nodes, m.deps = msg.nodes, msg.deps
		m.expandActive()
		m.index()
		return m, nil

	case verifiedMsg:
		m.verifying = false
		for i := range m.nodes {
			if m.nodes[i].ID == msg.id {
				m.nodes[i].LastResult = msg.res
				m.nodes[i].LastRun = "just now"
			}
		}
		m.index()
		switch msg.res {
		case store.VerifyPass:
			m.flash = "✓ condition passed · " + msg.id
		case store.VerifyFail:
			m.flash = "✕ condition failed · " + msg.id
		default:
			m.flash = "condition inconclusive · " + msg.id
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Overlays swallow keys first.
	switch m.mode {
	case modeFinder:
		return m.finderKey(msg)
	case modeStatus:
		return m.statusKey(key)
	case modeConfirm:
		return m.confirmKey(key)
	}

	if m.screen == screenHelp {
		if key == "?" || key == "esc" || key == "q" {
			m.screen = screenBrowser
		}
		return m, nil
	}

	if m.screen == screenProjects {
		return m.projectsKey(key)
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.screen = screenHelp
	case "p":
		m.screen = screenProjects
	case "j", "down":
		m.cursor = min(m.cursor+1, len(m.rows)-1)
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(len(m.rows)-1, 0)
	case "tab":
		m.focusDetail = !m.focusDetail
	case " ", "space":
		if n, ok := m.current(); ok && n.Type != store.TypeStep {
			m.expanded[n.ID] = !m.expanded[n.ID]
			m.buildRows()
		}
	case "enter":
		if n, ok := m.current(); ok {
			if n.Type == store.TypeWorkPackage {
				m.expanded[n.ID] = true
				m.buildRows()
			} else {
				m.focusDetail = true
			}
		}
	case "esc":
		m.focusDetail = false
		m.flash = ""
	case "a":
		m.showArchived = !m.showArchived
		m.buildRows()
	case "/", "ctrl+p":
		m.mode = modeFinder
		m.finder.SetValue("")
		m.results = nil
		m.fCursor = 0
		return m, m.finder.Focus()
	case "s":
		if n, ok := m.detailNode(); ok {
			m.mode = modeStatus
			m.statusCursor = statusIndex(n.Status)
		}
	case "d":
		if n, ok := m.detailNode(); ok && n.Status != store.StatusDone {
			m.pending = store.StatusDone
			m.mode = modeConfirm
		}
	case "v":
		if n, ok := m.detailNode(); ok && n.Condition != "" {
			m.verifying = true
			m.flash = ""
			return m, tea.Batch(verifyNode(m.st, n.ID), m.spin.Tick)
		}
	case "u":
		if m.last != nil {
			c := *m.last
			m.last = nil
			m.flash = "undid " + c.nodeID
			return m, applyStatus(m.st, m.projectID, c.nodeID, c.prev)
		}
	}
	return m, nil
}

func (m Model) projectsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.projCursor = min(m.projCursor+1, len(m.projects)-1)
	case "k", "up":
		m.projCursor = max(m.projCursor-1, 0)
	case "esc":
		if m.projectID != "" {
			m.screen = screenBrowser
		}
	case "enter":
		if m.projCursor < len(m.projects) {
			m.projectID = m.projects[m.projCursor].ID
			m.screen = screenBrowser
			m.cursor = 0
			return m, loadData(m.st, m.projectID)
		}
	}
	return m, nil
}

func (m Model) finderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNone
		m.finder.Blur()
		return m, nil
	case "down", "ctrl+n":
		m.fCursor = min(m.fCursor+1, max(len(m.results)-1, 0))
		return m, nil
	case "up", "ctrl+p":
		m.fCursor = max(m.fCursor-1, 0)
		return m, nil
	case "enter":
		if m.fCursor < len(m.results) {
			r := m.results[m.fCursor]
			if r.kind != "cmd" {
				m.jumpTo(r.node)
			}
		}
		m.mode = modeNone
		m.finder.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.finder, cmd = m.finder.Update(msg)
	m.search(m.finder.Value())
	if m.fCursor >= len(m.results) {
		m.fCursor = 0
	}
	return m, cmd
}

// jumpTo expands whatever it takes to put a node on screen, then parks the
// cursor on it.
func (m *Model) jumpTo(n store.Node) {
	if n.Type == store.TypeStep {
		if p, ok := m.byID[n.ParentID]; ok {
			m.expanded[p.ID] = true
			if wp, ok := m.byID[p.ParentID]; ok {
				m.expanded[wp.ID] = true
			}
		}
	} else if wp, ok := m.byID[n.ParentID]; ok {
		m.expanded[wp.ID] = true
	}
	m.buildRows()
	for i, r := range m.rows {
		if r.node.ID == n.ID {
			m.cursor = i
			return
		}
	}
}

func (m Model) statusKey(key string) (tea.Model, tea.Cmd) {
	pick := func(s store.Status) (tea.Model, tea.Cmd) {
		n, ok := m.detailNode()
		if !ok {
			m.mode = modeNone
			return m, nil
		}
		if s == store.StatusDone {
			m.pending = store.StatusDone
			m.mode = modeConfirm
			return m, nil
		}
		m.mode = modeNone
		m.last = &change{nodeID: n.ID, prev: n.Status}
		m.flash = n.ID + " → " + string(s) + " · u to undo"
		return m, applyStatus(m.st, m.projectID, n.ID, s)
	}

	switch key {
	case "esc":
		m.mode = modeNone
	case "j", "down":
		m.statusCursor = min(m.statusCursor+1, len(store.AllStatuses)-1)
	case "k", "up":
		m.statusCursor = max(m.statusCursor-1, 0)
	case "r":
		return pick(store.StatusReady)
	case "b":
		return pick(store.StatusBlocked)
	case "x":
		return pick(store.StatusDeferred)
	case "d":
		return pick(store.StatusDone)
	case "enter":
		return pick(store.AllStatuses[m.statusCursor])
	}
	return m, nil
}

func (m Model) confirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "n":
		m.mode = modeNone
	case "enter", "y":
		n, ok := m.detailNode()
		m.mode = modeNone
		if !ok {
			return m, nil
		}
		m.last = &change{nodeID: n.ID, prev: n.Status}
		m.flash = n.ID + " → DONE · u to undo · cascade is the core's job"
		return m, applyStatus(m.st, m.projectID, n.ID, m.pending)
	}
	return m, nil
}

func statusIndex(s store.Status) int {
	for i, v := range store.AllStatuses {
		if v == s {
			return i
		}
	}
	return 0
}
