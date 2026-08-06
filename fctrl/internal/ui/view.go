package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"fctrl/internal/store"
	"fctrl/internal/styles"
)

func key(s string) string { return styles.AccentS.Render(s) }

func pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// frame draws the terminal box: ┌─ title ─┐ … ├─┤ keys └─┘.
// Every row is padded to `inner` so the right wall closes.
func frame(title string, body []string, keys string, inner, height int) string {
	var b strings.Builder

	head := "┌─ " + title + " "
	dashes := inner + 2 - len(head) - 1
	if dashes < 0 {
		dashes = 0
	}
	b.WriteString(styles.AccentS.Render(head+strings.Repeat("─", dashes)+"┐") + "\n")

	// leave room for header, separator, key line and footer
	room := height - 4
	if room < 1 {
		room = 1
	}
	if len(body) > room {
		body = body[:room]
	}
	wall := styles.DimS.Render("│")
	for _, line := range body {
		visible := len([]rune(stripANSI(line)))
		fill := inner - visible
		if fill < 0 {
			fill = 0
		}
		b.WriteString(wall + " " + line + strings.Repeat(" ", fill) + " " + wall + "\n")
	}
	for i := len(body); i < room; i++ {
		b.WriteString(wall + strings.Repeat(" ", inner+2) + wall + "\n")
	}

	b.WriteString(styles.DimS.Render("├"+strings.Repeat("─", inner+2)+"┤") + "\n")
	kv := len([]rune(stripANSI(keys)))
	kfill := inner - kv
	if kfill < 0 {
		kfill = 0
	}
	b.WriteString(wall + " " + keys + strings.Repeat(" ", kfill) + " " + wall + "\n")
	b.WriteString(styles.AccentS.Render("└" + strings.Repeat("─", inner+2) + "┘"))
	return b.String()
}

func (m Model) View() string {
	if m.err != nil {
		return "\n  " + lipgloss.NewStyle().Foreground(styles.Blocked).Render("error: "+m.err.Error()) + "\n"
	}
	if len(m.nodes) == 0 {
		return "\n  loading…\n"
	}

	w := min(m.width-2, 200)
	h := m.height - 2
	if h < 12 {
		h = 12
	}

	var main string
	switch m.screen {
	case ScreenLanes:
		if m.width < OneLaneMin {
			main = m.viewTree(w, h)
		} else {
			main = m.viewLanes(w, h)
		}
	case ScreenChain:
		main = m.viewChain(w, h)
	case ScreenDetail:
		main = m.viewDetail(w, h)
	case ScreenActivity:
		main = m.viewActivity(w, h)
	default:
		main = m.viewTree(w, h)
	}

	if ov := m.viewOverlay(w); ov != "" {
		return main + "\n" + ov
	}
	if m.flash != "" {
		return main + "\n " + styles.DimS.Render(m.flash)
	}
	return main
}

func (m Model) viewTree(w, h int) string {
	inner := w - 4
	var body []string

	d, r, b, df, _ := m.projectTotals()
	summary := styles.Status(store.Ready).Render(fmt.Sprintf("READY %d", r)) + "   " +
		styles.Status(store.Blocked).Render(fmt.Sprintf("BLOCKED %d", b)) + "   " +
		styles.Status(store.Deferred).Render(fmt.Sprintf("DEFER %d", df)) + "   " +
		styles.Status(store.Done).Render(fmt.Sprintf("DONE %d", d))
	body = append(body, summary)
	body = append(body, styles.DimS.Render(strings.Repeat("─", inner)))

	for i, rw := range m.rows {
		sel := i == m.cursor
		if rw.isWP {
			caret := "▸"
			if rw.expanded {
				caret = "▾"
			}
			cd, cr, cb, cdf, total := m.counts(rw.node.ID)
			pct := 0
			if total > 0 {
				pct = cd * 100 / total
			}
			state := styles.DimS.Render(string(rw.node.State))
			if rw.node.State == store.Active {
				state = styles.Status(store.Ready).Render(string(rw.node.State))
			}
			bar := progressBar(cd, cr, cb, cdf, total, 14)
			line := styles.DimS.Render(caret) + " " + styles.BrightS.Render(rw.node.Title) + "  " + state + "  " + bar +
				styles.DimS.Render(fmt.Sprintf(" %d%%", pct))
			body = append(body, line)
			continue
		}

		t := rw.node
		done, total := m.stepRatio(t.ID)
		bdg := t.Verification.Badge()
		idS, titleS := styles.DimS, styles.FgS
		if sel {
			idS, titleS = styles.AccentS, styles.BrightS
		}
		if t.Status == store.Done {
			titleS = styles.DimS
		}
		cond := t.Condition
		if len(cond) > 22 {
			cond = cond[:22]
		}
		line := "  " + styles.Status(t.Status).Render("●") + " " + idS.Render(t.ID) + "  " +
			titleS.Render(padTrunc(t.Title, max(inner-52, 10))) + " " +
			styles.Status(bdg.Kind).Render(bdg.Glyph) + " " +
			styles.DimS.Render(padTrunc(cond, 22)) + " " +
			styles.DimS.Render(fmt.Sprintf("%d/%d", done, total))
		body = append(body, line)

		if bl := m.blockers[t.ID]; len(bl) > 0 && t.Status == store.Blocked {
			body = append(body, styles.DimS.Render("      ⊘ blocked by "+strings.Join(bl, ", ")))
		}
	}

	hidden := 0
	for _, wp := range m.workPackages() {
		if wp.State == store.WPDone || wp.State == store.Archived {
			hidden++
		}
	}
	if hidden > 0 && !m.showDone {
		body = append(body, styles.DimS.Render(fmt.Sprintf("▸ %d completed work packages", hidden)))
	}

	keys := key("j/k") + " move  " + key("h/l") + " fold  " + key("ret") + " detail  " +
		key("2") + " lanes  " + key("3") + " chain  " + key("/") + " find  " + key("s") + " status"
	title := "fctrl ─ " + m.projectName()
	return frame(title, body, keys, inner, h)
}

func (m Model) viewDetail(w, h int) string {
	inner := w - 4
	node, ok := m.byID[m.selectedID]
	if !ok {
		return m.viewTree(w, h)
	}
	var body []string

	crumb := m.projectName()
	if p, ok := m.byID[node.ParentID]; ok {
		crumb += " / " + p.Title
	}
	body = append(body, styles.DimS.Render(crumb))
	body = append(body, "")

	for _, p := range node.Description {
		for _, l := range wrapPlain(p, inner) {
			body = append(body, styles.FgS.Render(l))
		}
		body = append(body, "")
	}

	if node.Condition != "" {
		body = append(body, styles.DimS.Render("─ condition "+strings.Repeat("─", max(inner-12, 0))))
		body = append(body, styles.BrightS.Render(node.Condition))
		b := node.Verification.Badge()
		box := "[" + b.Glyph + "]"
		line := styles.Status(b.Kind).Render(box+" "+b.Label) + "   " + styles.DimS.Render(b.Detail)
		body = append(body, line)
		body = append(body, styles.DimS.Render("fctrl never runs conditions. The agent reports; you accept.  "+key("v")+" toggle"))
		body = append(body, "")
	}

	steps := m.stepsOf(node.ID)
	if len(steps) > 0 {
		done, total := m.stepRatio(node.ID)
		body = append(body, styles.DimS.Render(fmt.Sprintf("─ steps %d/%d ", done, total)+strings.Repeat("─", max(inner-14, 0))))
		for i, s := range steps {
			marker := " "
			nameS := styles.FgS
			if i == m.stepCursor {
				marker = "▸"
				nameS = styles.BrightS
			}
			if s.Status == store.Done {
				nameS = styles.DimS
			}
			fold := " "
			if s.Note != "" {
				fold = "⌄"
				if m.openSteps[s.ID] {
					fold = "^"
				}
			}
			body = append(body, styles.AccentS.Render(marker)+" "+
				styles.Status(s.Status).Render(styles.StepGlyph(s.Status))+" "+
				nameS.Render(padTrunc(s.Title, max(inner-8, 10)))+" "+styles.DimS.Render(fold))
			if m.openSteps[s.ID] && s.Note != "" {
				for _, l := range wrapPlain(s.Note, inner-6) {
					body = append(body, "     "+styles.DimS.Render(l))
				}
				if s.Condition != "" {
					body = append(body, "     "+styles.DimS.Render(s.Condition))
				}
			}
		}
		body = append(body, "")
	}

	if bl, blk := m.blockers[node.ID], m.blocks[node.ID]; len(bl)+len(blk) > 0 {
		body = append(body, styles.DimS.Render("─ deps "+strings.Repeat("─", max(inner-7, 0))))
		for _, id := range bl {
			body = append(body, depLine("blocked by", id, m))
		}
		for _, id := range blk {
			body = append(body, depLine("blocks", id, m))
		}
	}

	keys := key("s") + " status  " + key("v") + " verify flag  " + key("tab") + " expand step  " +
		key("a") + " activity  " + key("esc") + " back"
	title := node.ID + " ─ " + node.Title
	return frame(title, body, keys, inner, h)
}

func depLine(dir, id string, m Model) string {
	n, ok := m.byID[id]
	title := "outside this project"
	st := store.Deferred
	if ok {
		title, st = n.Title, n.Status
	}
	cross := ""
	if ok {
		if p, pok := m.byID[n.ParentID]; pok && p.ID != m.byID[m.selectedID].ParentID {
			cross = "  " + styles.DimS.Render("["+p.Title+"]")
		}
	}
	return styles.DimS.Render(padTrunc(dir, 11)) + " " + styles.Status(st).Render("●") + " " +
		styles.DimS.Render(id) + " " + styles.FgS.Render(title) + cross
}

func (m Model) viewActivity(w, h int) string {
	inner := w - 4
	node, ok := m.byID[m.selectedID]
	if !ok {
		return m.viewTree(w, h)
	}
	var body []string
	body = append(body, "")
	for _, a := range m.activity {
		if a.NodeID != node.ID {
			continue
		}
		colour := styles.Done
		switch a.Kind {
		case store.ActVerify:
			colour = styles.Ready
		case store.ActStatus:
			colour = styles.Accent
		case store.ActEdit:
			colour = styles.Deferred
		}
		body = append(body, lipgloss.NewStyle().Foreground(colour).Render("●")+" "+
			styles.BrightS.Render(a.Author)+"  "+styles.DimS.Render(a.When))
		for _, l := range wrapPlain(a.Text, inner-4) {
			body = append(body, styles.DimS.Render("│   ")+styles.FgS.Render(l))
		}
		body = append(body, styles.DimS.Render("│"))
	}
	body = append(body, "")
	body = append(body, styles.AccentS.Render("› ")+styles.DimS.Render("i to leave a note"))

	keys := key("i") + " write  " + key("j/k") + " scroll  " + key("esc") + " back to detail"
	return frame("activity ─ "+node.ID+" "+node.Title, body, keys, inner, h)
}

func (m Model) projectName() string {
	for _, p := range m.projects {
		if p.ID == m.projectID {
			return p.Name
		}
	}
	return m.projectID
}

func (m Model) projectTotals() (d, r, b, df, total int) {
	for _, n := range m.nodes {
		if n.Type == store.WorkPackage {
			continue
		}
		total++
		switch n.Status {
		case store.Done:
			d++
		case store.Ready:
			r++
		case store.Blocked:
			b++
		default:
			df++
		}
	}
	return
}

func progressBar(d, r, b, df, total, w int) string {
	if total == 0 {
		return styles.DimS.Render(strings.Repeat("░", w))
	}
	seg := func(n int) int { return n * w / total }
	dw, rw, bw := seg(d), seg(r), seg(b)
	rest := w - dw - rw - bw
	if rest < 0 {
		rest = 0
	}
	return styles.Status(store.Done).Render(strings.Repeat("█", dw)) +
		styles.Status(store.Ready).Render(strings.Repeat("█", rw)) +
		styles.Status(store.Blocked).Render(strings.Repeat("█", bw)) +
		styles.DimS.Render(strings.Repeat("█", rest))
}

func wrapPlain(s string, w int) []string {
	if w < 10 {
		w = 10
	}
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if len(line)+len(word)+1 > w {
			out = append(out, line)
			line = word
		} else if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
