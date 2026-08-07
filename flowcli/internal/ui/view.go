package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// wlen returns the display width (in terminal cells) of a string, counting
// multibyte glyphs like box corners and check marks as a single cell. Go's
// len() counts bytes, which misaligns the frame; this is the correct measure.
// ANSI sequences (colour codes in rendered lines) are stripped first.
func wlen(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// Shared fixed-width columns for the tree view's right tail, so the
// work-package step-ratio bar lines up under the task condition and the
// step-percent under the task step-counter.
const (
	condW  = 22 // condition text column / WP step-ratio bar width
	ratioW = 5  // task step-counter / WP step-percent column (e.g. "18/20", "90%")
)

func pad(s string, w int) string {
	if wlen(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-wlen(s))
}

// padr right-aligns s within `w` cells (padding on the left). Used to line up
// the WP step-percent with the task step-counter in the rightmost column so
// both rows' numbers end at the same column.
func padr(s string, w int) string {
	if wlen(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-wlen(s)) + s
}

// treeRow lays out a tree line so the styled `right` tail is flush to the
// right edge of the frame (inner cells wide). `prefix` is styled text;
// `titlePlain` is the unstyled title and `titleStyled` its styled version.
// The title is padded/truncated so prefix + title + right exactly fills inner
// cells, keeping the right tail right-aligned on every row.
func treeRow(prefix, titlePlain, titleStyled, right string, inner int) string {
	tw := inner - wlen(prefix) - wlen(right)
	if tw < 1 {
		tw = 1
	}
	return prefix + cell(titlePlain, titleStyled, tw) + right
}

// frame draws the terminal box: ┌─ title ─┐ … ├─┤ keys └─┘.
// Every row is padded to `inner` so the right wall closes.
func frame(title string, body []string, keys string, inner, height int) string {
	var b strings.Builder

	head := "╭─ " + title + " "
	dashes := inner + 4 - wlen(head) - 1
	if dashes < 0 {
		dashes = 0
	}
	b.WriteString(styles.AccentS.Render(head+strings.Repeat("─", dashes)+"╮") + "\n")

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
		visible := wlen(line)
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
	kv := wlen(keys)
	kfill := inner - kv
	if kfill < 0 {
		kfill = 0
	}
	b.WriteString(wall + " " + keys + strings.Repeat(" ", kfill) + " " + wall + "\n")
	b.WriteString(styles.AccentS.Render("╰" + strings.Repeat("─", inner+2) + "╯"))
	return b.String()
}

func (m Model) View() string {
	if m.err != nil {
		return "\n  " + lipgloss.NewStyle().Foreground(styles.Blocked).Render("error: "+m.err.Error()) + "\n"
	}
	if len(m.nodes) == 0 {
		return "\n  loading…\n"
	}

	w := m.width
	h := m.height
	// Guard against a zero/unknown terminal width: layout math (e.g.
	// strings.Repeat) panics on a negative count. A narrow-but-usable floor
	// beats a crash.
	if w < 20 {
		w = 20
	}
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
		return overlay(main, ov, w, h)
	}
	if m.flash != "" {
		return main + "\n " + styles.DimS.Render(m.flash)
	}
	return main
}

func (m Model) viewTree(w, h int) string {
	inner := w - 4
	var body []string

	d, r, b, df, total := m.projectTotals()
	pct := 0
	if total > 0 {
		pct = d * 100 / total
	}
	prefix := "  " +
		styles.Status(store.Ready).Render(fmt.Sprintf("READY %d", r)) + "   " +
		styles.Status(store.Blocked).Render(fmt.Sprintf("BLOCKED %d", b)) + "   " +
		styles.Status(store.Deferred).Render(fmt.Sprintf("DEFER %d", df)) + "   " +
		styles.Status(store.Done).Render(fmt.Sprintf("DONE %d", d))
	// Project tail mirrors the WP row: step-ratio bar under the task condition
	// column, step-percent right-aligned under the task step-counter.
	bar := progressBar(d, r, b, df, total, condW)
	right := "  " + bar + " " + padr(fmt.Sprintf("%d%%", pct), ratioW)
	summary := treeRow(prefix, "", "", right, inner)
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
			bar := progressBar(cd, cr, cb, cdf, total, condW)
			prefix := styles.DimS.Render(caret) + " "
			// WP columns share the same fixed widths as the task row so the
			// step-ratio bar lands under the task condition column and the
			// step-percent under the task step-counter.
			right := "  " + state + "  " + bar + " " + padr(fmt.Sprintf("%d%%", pct), ratioW)
			line := treeRow(prefix, rw.node.Title, styles.BrightS.Render(rw.node.Title), right, inner)
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
		prefix := "  " + styles.Status(t.Status).Render("●") + " " + idS.Render(t.ID) + "  "
		condS := styles.DimS.Render(pad(cond, condW))
		ratioS := padr(fmt.Sprintf("%d/%d", done, total), ratioW)
		right := " " + styles.Status(bdg.Kind).Render(bdg.Glyph) + " " + condS + " " + ratioS
		line := treeRow(prefix, t.Title, titleS.Render(t.Title), right, inner)
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

	keys := m.statusLine(treeKeys(), m.screen, inner)
	title := "flowcli ─ " + m.projectName()
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
		body = append(body, styles.DimS.Render("fctrl never runs conditions. The agent reports; you accept.  "+styles.AccentS.Render("v")+" toggle"))
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

	keys := m.statusLine(detailKeys(), m.screen, inner)
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

	keys := m.statusLine(activityKeys(), m.screen, inner)
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
