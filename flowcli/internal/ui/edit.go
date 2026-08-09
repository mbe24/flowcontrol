package ui

// inline edit of a node's title and condition (designer phase E: "c in
// detail"). Reuses the Form primitive and the boxWithKeys dialog frame.

import (
	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// editWidth is the fixed content width of the edit dialog, matching the other
// form dialogs.
const editWidth = 44

// editSpecs returns the two fields of the inline editor, prefilled with the
// node's current values.
func editSpecs(title, cond string) []fieldSpec {
	return []fieldSpec{
		{label: "title", placeholder: "Short imperative summary", value: title},
		{label: "condition", hint: "optional · the agent verifies this", placeholder: "pnpm test:auth --grep rotate", value: cond, mono: true},
	}
}

// openEdit starts the inline editor for the given node, copying its current
// title and condition into the form.
func (m *Model) openEdit(nodeID string) {
	node, ok := m.byID[nodeID]
	if !ok {
		return
	}
	m.edit = editState{
		nodeID: nodeID,
		form:   newForm(editWidth, editSpecs(node.Title, node.Condition)),
		errAt:  -1,
	}
	m.overlay = OverlayEdit
}

func (m *Model) closeEdit() {
	m.edit = editState{errAt: -1}
	m.overlay = OverlayNone
}

func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.edit
	switch msg.String() {
	case "esc":
		m.closeEdit()
		return m, nil

	case "tab":
		c.form.Next()
		return m, nil
	case "shift+tab":
		c.form.Prev()
		return m, nil

	case "enter":
		return m.submitEdit()

	case "ctrl+s":
		return m.submitEdit()
	}

	cmd := c.form.Update(msg)
	return m, cmd
}

func (m Model) submitEdit() (tea.Model, tea.Cmd) {
	c := &m.edit
	node, ok := m.byID[c.nodeID]
	if !ok {
		m.closeEdit()
		return m, nil
	}
	title := c.form.Value(0)
	if title == "" {
		c.errAt = 0
		c.form.err = "a title is required"
		return m, nil
	}
	c.errAt = -1
	c.form.err = ""

	nodeID := c.nodeID
	cond := c.form.Value(1)

	var upd store.NodeUpdate
	if title != node.Title {
		upd.Title = &title
	}
	if cond != node.Condition {
		upd.Condition = &cond
	}

	m.closeEdit()

	if upd.Title == nil && upd.Condition == nil {
		// nothing changed — just leave the editor
		return m, nil
	}

	return m, func() tea.Msg {
		if err := m.store.UpdateNode(m.ctx, nodeID, upd); err != nil {
			return refreshedMsg{err: err}
		}
		return m.refresh()
	}
}

func (m Model) viewEdit(w int) string {
	c := m.edit
	node, ok := m.byID[c.nodeID]
	if !ok {
		return ""
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+styles.BrightS.Render(node.ID+"  "+node.Title))
	lines = append(lines, "")
	lines = append(lines, c.form.Rows(c.errAt)...)
	// The stale note is only truthful once the condition field changes. Show
	// it under the form whenever the condition differs from what was loaded,
	// so the designer's "say so in the dialog" rule holds without noise.
	if c.form.Value(1) != node.Condition {
		lines = append(lines, "",
			"  "+styles.DimS.Render("! editing the condition marks the agent report stale"))
	}

	keys := styles.AccentS.Render("⇥") + styles.DimS.Render(" field  ") +
		styles.AccentS.Render("↵") + styles.DimS.Render(" save  ") +
		styles.AccentS.Render("esc") + styles.DimS.Render(" cancel")

	title := "edit " + node.ID
	return boxWithKeys(title, styles.Accent, lines, keys, editWidth, w)
}