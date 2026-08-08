package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// effect is one downstream consequence of a pending status change.
type effect struct {
	node store.Node
	from store.Status
	to   store.Status
	// crossWP names the owning package when it differs from the subject's.
	crossWP string
	// stuck marks a dependent that stays blocked by something else. Listing it
	// matters: showing only the wins would make the dialog a liar.
	stuck bool
}

// cascadeState backs the "mark done?" preview. It is only ever opened when
// there is something to preview — a change with no downstream effect applies
// immediately and reports in the status line.
type cascadeState struct {
	active   bool
	nodeID   string
	from, to store.Status

	effects []effect
	scroll  int

	// openSteps is the count of the subject's own steps that are not DONE.
	// Marking a task done with steps still open asks whether to close them.
	openSteps  int
	closeSteps bool
	// stage 0 = review the cascade, 1 = the close-steps question.
	stage int
}

const cascadeWidth = 46
const cascadeVisible = 4

// planCascade computes what a status change would do. Returns ok=false when
// there is nothing to preview, in which case the caller applies immediately.
func (m Model) planCascade(n store.Node, to store.Status) (cascadeState, bool) {
	c := cascadeState{active: true, nodeID: n.ID, from: n.Status, to: to}

	if to == store.Done {
		for _, s := range m.stepsOf(n.ID) {
			if s.Status != store.Done {
				c.openSteps++
			}
		}
		c.closeSteps = true
	}

	// Direct dependents only: a node is blocked if any direct blocker is not
	// done, and that blocker's own blockers are its problem.
	for _, id := range m.blocks[n.ID] {
		dep, ok := m.byID[id]
		if !ok || dep.Status != store.Blocked {
			continue
		}
		e := effect{node: dep, from: dep.Status, to: store.Ready}
		if to != store.Done {
			continue
		}
		// Still blocked by someone else?
		for _, b := range m.blockers[dep.ID] {
			if b == n.ID {
				continue
			}
			if other, ok := m.byID[b]; ok && other.Status != store.Done {
				e.stuck = true
				e.to = dep.Status
				break
			}
		}
		if p, ok := m.byID[dep.ParentID]; ok && dep.ParentID != n.ParentID {
			e.crossWP = p.Title
		}
		c.effects = append(c.effects, e)
	}

	// A work package going done also closes its children — a different kind of
	// consequence, counted separately in the header.
	if n.Type == store.WorkPackage && to == store.Done {
		for _, t := range m.childrenOf(n.ID, store.Task) {
			if t.Status != store.Done {
				c.effects = append(c.effects, effect{node: t, from: t.Status, to: store.Done})
			}
		}
	}

	return c, len(c.effects) > 0 || c.openSteps > 0
}

func (m Model) updateCascade(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.cascade
	switch msg.String() {
	case "esc", "n":
		if c.stage == 1 && msg.String() == "n" {
			// "close them too? [y/N]" — N is the default, apply without.
			c.closeSteps = false
			return m.applyCascade()
		}
		m.cascade = cascadeState{}
		m.overlay = OverlayNone
		return m, nil

	case "j", "down":
		c.scroll = min(c.scroll+1, max(0, len(c.effects)-cascadeVisible))
	case "k", "up":
		c.scroll = max(c.scroll-1, 0)

	case "y":
		if c.stage == 1 {
			c.closeSteps = true
			return m.applyCascade()
		}

	case "enter":
		// Steps still open is a second question, asked after the cascade is
		// reviewed rather than crammed into the same screen.
		if c.stage == 0 && c.openSteps > 0 {
			c.stage = 1
			return m, nil
		}
		return m.applyCascade()
	}
	return m, nil
}

func (m Model) applyCascade() (tea.Model, tea.Cmd) {
	c := m.cascade
	id, to, closeSteps := c.nodeID, c.to, c.closeSteps
	var stepIDs []string
	if closeSteps {
		for _, s := range m.stepsOf(id) {
			if s.Status != store.Done {
				stepIDs = append(stepIDs, s.ID)
			}
		}
	}
	n := len(c.effects)

	m.cascade = cascadeState{}
	m.overlay = OverlayNone
	m.lastStatus = &struct {
		id   string
		prev store.Status
	}{id, c.from}
	m.flash = fmt.Sprintf("%s → %s · %d affected", id, to, n)

	return m, func() tea.Msg {
		// One key committed this, so one undo reverses it: the engine owns the
		// cascade, we only write the nodes the operator agreed to.
		for _, sid := range stepIDs {
			if err := m.store.SetStatus(m.ctx, sid, store.Done); err != nil {
				return refreshedMsg{err: err}
			}
		}
		if err := m.store.SetStatus(m.ctx, id, to); err != nil {
			return refreshedMsg{err: err}
		}
		return m.refresh()
	}
}

func (m Model) viewCascade(w int) string {
	c := m.cascade
	n, ok := m.byID[c.nodeID]
	if !ok {
		return ""
	}

	// Stage 1: the steps question, on its own so the answer is unambiguous.
	if c.stage == 1 {
		lines := []string{
			"",
			styles.FgS.Render(fmt.Sprintf("%d %s still open on %s.",
				c.openSteps, plural(c.openSteps, "step", "steps"), n.ID)),
			"",
			styles.DimS.Render("Close them too, or leave them open and mark"),
			styles.DimS.Render("only the task done?"),
			"",
		}
		keys := styles.AccentS.Render("y") + styles.DimS.Render(" close all  ") +
			styles.AccentS.Render("n") + styles.DimS.Render(" leave open  ") +
			styles.AccentS.Render("esc") + styles.DimS.Render(" cancel")
		return boxWithKeys("close "+plural(c.openSteps, "step", "steps")+"?", styles.Ready, lines, keys, cascadeWidth, w)
	}

	var lines []string
	lines = append(lines, "")

	mark := "●"
	if n.Type == store.WorkPackage {
		mark = "▪"
	}
	lines = append(lines,
		styles.Status(n.Status).Render(mark)+" "+styles.FgS.Render(n.ID+"  "+n.Title))
	lines = append(lines,
		"  "+styles.Status(c.from).Render(string(c.from))+styles.DimS.Render(" → ")+
			styles.Status(c.to).Render(string(c.to)))
	lines = append(lines, "")

	// Header rule counts both directions.
	unblocks, closes := 0, 0
	for _, e := range c.effects {
		if e.stuck {
			continue
		}
		if e.to == store.Ready {
			unblocks++
		} else if e.to == store.Done {
			closes++
		}
	}
	head := fmt.Sprintf("unblocks %d", unblocks)
	if closes > 0 {
		head += fmt.Sprintf(" · closes %d", closes)
	}
	page := ""
	if len(c.effects) > cascadeVisible {
		page = fmt.Sprintf("%d/%d", c.scroll/cascadeVisible+1,
			(len(c.effects)+cascadeVisible-1)/cascadeVisible)
	}
	lines = append(lines, sectionRule(head, page, cascadeWidth))
	lines = append(lines, "")

	end := min(c.scroll+cascadeVisible, len(c.effects))
	for _, e := range c.effects[c.scroll:end] {
		trans := styles.Status(e.from).Render(string(e.from)) +
			styles.DimS.Render(" → ") + styles.Status(e.to).Render(string(e.to))
		if e.stuck {
			trans = styles.DimS.Render("still blocked")
		}
		glyph := styles.Status(e.node.Status).Render("●")
		if e.stuck {
			glyph = styles.S.Copy().Foreground(styles.Deferred).Render("◇")
		}
		// title uses (almost) the full dialog width
		title := padTrunc(e.node.ID+" "+e.node.Title, cascadeWidth-8)
		lines = append(lines, styles.DimS.Render("└ ")+glyph+" "+styles.FgS.Render(title))
		// cross-WP name below the task, then the status verdict below that
		// (order: task, WP if cross-dep, status verdict)
		if e.crossWP != "" {
			lines = append(lines, "    "+styles.S.Copy().Foreground(styles.Hues[1]).Render("⟨"+e.crossWP+"⟩"))
		}
		lines = append(lines, "    "+trans)
	}
	if len(c.effects) > end {
		lines = append(lines, "  "+styles.DimS.Render(fmt.Sprintf("▼ %d more", len(c.effects)-end)))
	}
	lines = append(lines, "")

	applyLabel := " apply"
	if len(c.effects) > 0 {
		applyLabel = fmt.Sprintf(" apply all %d", len(c.effects)+1)
	}
	if c.openSteps > 0 {
		applyLabel = " next"
	}
	keys := styles.AccentS.Render("↵") + styles.DimS.Render(applyLabel+"  ") +
		styles.AccentS.Render("j/k") + styles.DimS.Render(" scroll  ") +
		styles.AccentS.Render("esc") + styles.DimS.Render(" cancel")

	title := "mark " + strings.ToLower(string(c.to)) + "?"
	return boxWithKeys(title, styles.StatusColor(c.to), lines, keys, cascadeWidth, w)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
