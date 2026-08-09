package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
)

// depCand is one candidate blocker in the dependency-add search: cycle is true
// when adding it would close a loop (the current node already reaches it).
type depCand struct {
	node  store.Node
	cycle bool
}

// depRows returns the dependency edges touching a node, ordered blocked-by
// first (X ← B) then blocks (X → D). The detail deps cursor indexes into this
// flat list.
func (m Model) depRows(id string) []store.Dependency {
	var out []store.Dependency
	for _, d := range m.deps {
		if d.BlockedID == id {
			out = append(out, d)
		}
	}
	for _, d := range m.deps {
		if d.BlockerID == id {
			out = append(out, d)
		}
	}
	return out
}

// depRowLabel resolves how a dependency edge relates to the node: whether the
// other end blocks the node or is blocked by it.
func depRowLabel(d store.Dependency, id string) (dir, other string) {
	if d.BlockedID == id {
		return "blocked by", d.BlockerID
	}
	return "blocks", d.BlockedID
}

// depRemoveLabel is the far-side node of an edge being removed, shown alone in
// the remove-confirm dialog: the host node already names itself in the detail
// header, so only the other endpoint is repeated here.
func depRemoveLabel(m Model, d store.Dependency) string {
	_, other := depRowLabel(d, m.selectedID)
	if n, ok := m.byID[other]; ok {
		return other + "  " + n.Title
	}
	return other
}

// reaches reports whether start can reach target by following blocks edges.
func (m Model) reaches(start, target string) bool {
	seen := map[string]bool{start: true}
	stack := []string{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range m.blocks[cur] {
			if next == target {
				return true
			}
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return false
}

// buildDepCands lists project nodes as candidate blockers for id: id itself
// and its existing direct blockers are excluded; nodes the current node
// already reaches (adding them would close a cycle) are marked so the
// renderer can dim and skip them.
func (m *Model) buildDepCands(id string) {
	q := strings.ToLower(strings.TrimSpace(m.depQuery))
	m.depCands = m.depCands[:0]
	for _, n := range m.nodes {
		if n.ProjectID != m.projectID || n.ID == id {
			continue
		}
		dup := false
		for _, d := range m.deps {
			if d.BlockedID == id && d.BlockerID == n.ID {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(n.ID+" "+n.Title), q) {
			continue
		}
		m.depCands = append(m.depCands, depCand{node: n, cycle: m.reaches(id, n.ID)})
	}
	// land on the first addable (non-cycle) candidate
	for i, c := range m.depCands {
		if !c.cycle {
			m.depIdx = i
			break
		}
	}
	if m.depIdx >= m.depScroll+depVisible {
		m.depScroll = m.depIdx - depVisible + 1
	}
}

// depSelect moves the cursor to the nearest addable (non-cycle) candidate in
// dir (+1/-1), staying put when no addable candidate lies in that direction.
func (m *Model) depSelect(dir int) {
	n := len(m.depCands)
	if n == 0 {
		return
	}
	for i := 1; i < n; i++ {
		j := m.depIdx + dir*i
		if j < 0 || j >= n {
			break
		}
		if !m.depCands[j].cycle {
			m.depIdx = j
			return
		}
	}
}

// depVisible is how many candidate rows the dependency-add dialog shows.
const depVisible = 6

// updateDepAdd is the search dialog for adding a dependency: type to filter
// candidate blockers (title/id), up/down to move (cycle candidates are
// skipped), enter to add, esc to cancel.
func (m Model) updateDepAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	id := m.selectedID
	switch msg.String() {
	case "esc":
		m.overlay = OverlayNone
		m.input.Blur()
		return m, nil
	case "down", "ctrl+n":
		m.depSelect(1)
		if m.depIdx >= m.depScroll+depVisible {
			m.depScroll = m.depIdx - depVisible + 1
		}
		return m, nil
	case "up", "ctrl+p":
		m.depSelect(-1)
		if m.depIdx < m.depScroll {
			m.depScroll = m.depIdx
		}
		return m, nil
	case "enter":
		if m.depIdx < len(m.depCands) && !m.depCands[m.depIdx].cycle {
			cand := m.depCands[m.depIdx].node.ID
			m.overlay = OverlayNone
			m.input.Blur()
			return m, m.addDep(cand, id)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.depQuery = m.input.Value()
	m.buildDepCands(id)
	m.depIdx = 0
	m.depScroll = 0
	return m, cmd
}

// addDep records blocker → blocked and refreshes.
func (m Model) addDep(blocker, blocked string) tea.Cmd {
	return func() tea.Msg {
		if err := m.store.AddDependency(m.ctx, blocker, blocked); err != nil {
			return refreshedMsg{err: err}
		}
		return m.refresh()
	}
}

// removeDep drops the blocker → blocked edge and refreshes.
func (m Model) removeDep(blocker, blocked string) tea.Cmd {
	return func() tea.Msg {
		if err := m.store.RemoveDependency(m.ctx, blocker, blocked); err != nil {
			return refreshedMsg{err: err}
		}
		return m.refresh()
	}
}

// updateDepRemove is the remove-edge confirm dialog: esc cancels (clears the
// pending edge), y/enter removes it.
func (m Model) updateDepRemove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n", "q":
		m.depRemove = nil
		m.overlay = OverlayNone
		return m, nil
	case "y", "enter":
		d := m.depRemove
		m.depRemove = nil
		m.overlay = OverlayNone
		if d == nil {
			return m, nil
		}
		m.depFocus = false
		return m, m.removeDep(d.BlockerID, d.BlockedID)
	}
	return m, nil
}