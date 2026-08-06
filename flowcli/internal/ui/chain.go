package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// chainRow is one line of the dependency spine. Prefix carries the box-drawing
// gutter; the status is the dot's colour and nothing else.
type chainRow struct {
	node    store.Node
	prefix  string
	note    string
	crossWP string
	spacer  bool
}

func (m *Model) activeWPs() []store.Node {
	var out []store.Node
	for _, wp := range m.workPackages() {
		if wp.State == store.Active {
			out = append(out, wp)
		}
	}
	if len(out) == 0 {
		out = m.workPackages()
	}
	return out
}

// buildChain lays out the selected work package in dependency order, or the
// single path through the focused node.
func (m *Model) buildChain() {
	m.chainRows = nil
	wps := m.activeWPs()
	if len(wps) == 0 {
		return
	}
	if m.chainWP >= len(wps) {
		m.chainWP = 0
	}
	if m.focusID != "" {
		m.buildFocus()
		return
	}

	wp := wps[m.chainWP]
	tasks := m.childrenOf(wp.ID, store.Task)
	own := map[string]bool{}
	for _, t := range tasks {
		own[t.ID] = true
	}

	// roots: tasks with no blocker inside this package
	var roots []store.Node
	for _, t := range tasks {
		isRoot := true
		for _, b := range m.blockers[t.ID] {
			if own[b] {
				isRoot = false
			}
		}
		if isRoot {
			roots = append(roots, t)
		}
	}

	seen := map[string]bool{}
	var walk func(n store.Node, gutter string, last bool, depth int)
	walk = func(n store.Node, gutter string, last bool, depth int) {
		if seen[n.ID] {
			return
		}
		seen[n.ID] = true

		var prefix string
		if depth == 0 {
			prefix = ""
		} else if last {
			prefix = gutter + "└─"
		} else {
			prefix = gutter + "├─"
		}

		// children inside this package
		var kids []store.Node
		for _, b := range m.blocks[n.ID] {
			if own[b] && !seen[b] {
				if k, ok := m.byID[b]; ok {
					kids = append(kids, k)
				}
			}
		}
		if depth == 0 && len(kids) > 0 {
			prefix = "─┬"
		}

		// cross-package and rolled-up blockers become an annotation, not an edge
		var notes []string
		for _, b := range m.blockers[n.ID] {
			bn, ok := m.byID[b]
			if !ok {
				continue
			}
			if bn.Type == store.WorkPackage {
				notes = append(notes, fmt.Sprintf("also waits on [%s] %s", bn.Title, bn.State))
			} else if !own[b] {
				if p, ok := m.byID[bn.ParentID]; ok {
					notes = append(notes, fmt.Sprintf("also waits on %s [%s]", bn.ID, p.Title))
				}
			}
		}
		cross := ""
		if p, ok := m.byID[n.ParentID]; ok && p.ID != wp.ID {
			cross = "[" + p.Title + "]"
		}

		m.chainRows = append(m.chainRows, chainRow{node: n, prefix: prefix, crossWP: cross})
		for _, nt := range notes {
			childGutter := gutter
			if depth > 0 {
				if last {
					childGutter = gutter + "  "
				} else {
					childGutter = gutter + "│ "
				}
			}
			m.chainRows = append(m.chainRows, chainRow{prefix: childGutter + "   ", note: nt})
		}

		nextGutter := gutter
		if depth == 0 {
			nextGutter = "  "
		} else if last {
			nextGutter = gutter + "  "
		} else {
			nextGutter = gutter + "│ "
		}
		for i, k := range kids {
			walk(k, nextGutter, i == len(kids)-1, depth+1)
		}
	}

	for i, r := range roots {
		if i > 0 {
			m.chainRows = append(m.chainRows, chainRow{spacer: true})
		}
		walk(r, "", true, 0)
	}
	if m.chainCursor >= len(m.chainRows) {
		m.chainCursor = max(0, len(m.chainRows)-1)
	}
}

// buildFocus shows only the ancestors and descendants of one node.
func (m *Model) buildFocus() {
	target, ok := m.byID[m.focusID]
	if !ok {
		m.focusID = ""
		m.buildChain()
		return
	}
	var up []store.Node
	var climb func(id string, depth int)
	seen := map[string]bool{}
	climb = func(id string, depth int) {
		if depth > 6 {
			return
		}
		for _, b := range m.blockers[id] {
			if seen[b] {
				continue
			}
			seen[b] = true
			if bn, ok := m.byID[b]; ok {
				climb(b, depth+1)
				up = append(up, bn)
			}
		}
	}
	climb(target.ID, 0)

	m.chainRows = append(m.chainRows, chainRow{spacer: true, note: "upstream — what has to finish first"})
	for _, n := range up {
		m.chainRows = append(m.chainRows, chainRow{node: n})
		m.chainRows = append(m.chainRows, chainRow{spacer: true, prefix: "│"})
	}
	m.chainRows = append(m.chainRows, chainRow{node: target, prefix: "■"})

	var down []store.Node
	for _, b := range m.blocks[target.ID] {
		if n, ok := m.byID[b]; ok {
			down = append(down, n)
		}
	}
	if len(down) == 0 {
		m.chainRows = append(m.chainRows, chainRow{spacer: true, note: "· nothing waits on this"})
	} else {
		for _, n := range down {
			m.chainRows = append(m.chainRows, chainRow{spacer: true, prefix: "│"})
			m.chainRows = append(m.chainRows, chainRow{node: n})
		}
	}
	m.chainRows = append(m.chainRows, chainRow{spacer: true,
		note: fmt.Sprintf("%d hops from ready · longest chain %d", len(up), len(up)+len(down))})
}

func (m Model) viewChain(w, h int) string {
	inner := w - 4
	var body []string

	title := "chain"
	if m.focusID != "" {
		title = "chain ─ focus " + m.focusID
	} else if wps := m.activeWPs(); len(wps) > 0 {
		title = "chain ─ " + wps[m.chainWP].Title
	}

	body = append(body, "")
	for i, r := range m.chainRows {
		if r.spacer && r.node.ID == "" {
			line := styles.DimS.Render(r.prefix)
			if r.note != "" {
				line = styles.DimS.Render(r.prefix + r.note)
			}
			body = append(body, line)
			continue
		}
		if r.node.ID == "" {
			body = append(body, styles.DimS.Render(r.prefix+"╎ "+r.note))
			continue
		}
		sel := i == m.chainCursor
		dot := styles.Status(r.node.Status).Render("●")
		if r.prefix == "■" {
			dot = styles.Status(r.node.Status).Render("■")
		}
		idS := styles.DimS
		titleS := styles.FgS
		if sel {
			idS = styles.AccentS
			titleS = styles.BrightS
		}
		gutter := r.prefix
		if gutter == "■" {
			gutter = ""
		}
		line := styles.DimS.Render(gutter) + dot + " " + idS.Render(r.node.ID) + "  " + titleS.Render(r.node.Title)
		if r.crossWP != "" {
			line += "  " + lipgloss.NewStyle().Foreground(styles.Hues[1]).Render(r.crossWP)
		}
		if sel {
			line = styles.SelS.Render(pad(stripANSI(line), inner))
			line = styles.SelS.Render(gutter) + dot + " " + styles.AccentS.Render(r.node.ID) + "  " + styles.BrightS.Render(r.node.Title)
		}
		body = append(body, line)
	}
	body = append(body, "")

	keys := key("j/k") + " move  " + key("f") + " focus  " + key("w") + " next package  " +
		key("ret") + " detail  " + key("1") + " tree"
	return frame(title, body, keys, inner, h)
}

func stripANSI(s string) string {
	var out strings.Builder
	skip := false
	for _, r := range s {
		if r == '\x1b' {
			skip = true
			continue
		}
		if skip {
			if r == 'm' {
				skip = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
