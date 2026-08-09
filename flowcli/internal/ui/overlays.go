package ui

import (
	"fmt"
	"strconv"
	"strings"

	lv2 "charm.land/lipgloss/v2"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// deleteLines caps how many lines of the unblocked section the delete confirm
// shows at once (about one dependent); the rest is reached by scrolling with
// j/k (like the lanes).
const deleteLines = 5

// box draws a small centred overlay panel below the main screen.
func box(title, titleColour string, lines []string, w int) string {
	inner := 0
	for _, l := range lines {
		if v := len([]rune(stripANSI(l))); v > inner {
			inner = v
		}
	}
	if v := len([]rune(title)); v+4 > inner {
		inner = v + 4
	}
	if inner > w-6 {
		inner = w - 6
	}
	accent := styles.AccentS
	if titleColour == "blocked" {
		accent = styles.S.Copy().Foreground(styles.Blocked)
	}

	var b strings.Builder
	head := "╭─ " + title + " " // the header line must match the body width (inner+4: wall + space +
	// content + space + wall). `len` counts bytes and box glyphs are
	// multibyte, so compute dashes from the display width wlen(head); the
	// "+1" accounts for the ╮ corner.
	dashes := max(inner+4-wlen(head)-1, 0)
	b.WriteString(accent.Render(head+strings.Repeat("─", dashes)+"╮") + "\n")
	wall := styles.DimS.Render("│")
	for _, l := range lines {
		fill := inner - len([]rune(stripANSI(l)))
		if fill < 0 {
			fill = 0
		}
		b.WriteString(wall + " " + l + strings.Repeat(" ", fill) + " " + wall + "\n")
	}
	b.WriteString(accent.Render("╰" + strings.Repeat("─", inner+2) + "╯"))
	return b.String()
}

// overlay positions a dialog box as a modal centred over the full-screen main
// view using lipgloss v2's cell-based compositor. The main view is added as a
// background layer (opaque where it has content) and the dialog as a
// transparent foreground layer on top, so the underlying view stays fully
// visible around the dialog instead of being blanked out.
func overlay(main, ov string, w, h int) string {
	ovLines := strings.Split(strings.TrimRight(ov, "\n"), "\n")
	ovW := 0
	for _, l := range ovLines {
		if v := wlen(l); v > ovW {
			ovW = v
		}
	}
	ovH := len(ovLines)

	x := max((w-ovW)/2, 0)
	y := max((h-ovH)/2, 0)

	bg := lv2.NewLayer(main).X(0).Y(0).Z(0)
	dlg := lv2.NewLayer(ov).X(x).Y(y).Z(1)
	comp := lv2.NewCompositor(bg, dlg)
	return comp.Render()
}

func (m Model) viewOverlay(w int) string {
	switch m.overlay {
	case OverlayCreate:
		return m.viewCreate(w)
	case OverlayEdit:
		return m.viewEdit(w)
	case OverlayCascade:
		return m.viewCascade(w)
	case OverlayDelete:
		c := m.deleteInfo
		if c == nil {
			return ""
		}
		// Fixed content width (like the cascade dialog's cascadeWidth) so the
		// dialog is wide and the right-aligned tag column sits near the right
		// wall instead of in the screen center.
		const inner = 46

		var body []string
		body = append(body, "",
			"  "+styles.BrightS.Render(c.id+"  "+c.title),
			"")
		// Which lines describe the collateral depends on the kind of node:
		// a work package's descendants are tasks + steps; a task's are steps.
		// Numbers are left-aligned in a fixed-width column so the label after
		// them starts at the same column even as the digit count changes.
		pn := func(n int) string { return pad(fmt.Sprintf("%d", n), countW) }
		var descLines []string
		if m.byID[c.id].Type == store.WorkPackage {
			descLines = []string{
				pn(c.taskCount) + " task nodes",
				pn(c.stepCount) + " step nodes",
			}
		} else {
			descLines = []string{
				pn(c.stepCount) + " step nodes",
			}
		}
		// Right-aligned tag per line, mirroring the status dialog: every tag
		// ends flush against the right wall of the fixed-width dialog.
		type tagLine struct{ left, tag string }
		var tags []tagLine
		for _, l := range descLines {
			tags = append(tags, tagLine{left: l, tag: "deleted"})
		}
		tags = append(tags,
			tagLine{left: pn(c.edgeCount) + " dependency edges", tag: "removed"},
			tagLine{left: pn(c.actCount) + " activity entries", tag: "kept"},
		)
		// All tags share one column: each tag is right-aligned inside a
		// fixed-width field as wide as the longest tag ("removed"), so shorter
		// tags like "kept" still end at the same right edge.
		tagW := 0
		for _, tl := range tags {
			if v := wlen(tl.tag); v > tagW {
				tagW = v
			}
		}
		for _, tl := range tags {
			left := "  " + styles.FgS.Render(tl.left)
			tag := styles.S.Copy().Foreground(styles.Blocked).Render(tl.tag)
			if tl.tag != "deleted" {
				tag = styles.DimS.Render(tl.tag)
			}
			pad := inner - 2 - wlen(stripANSI(left)) - tagW
			if pad < 1 {
				pad = 1
			}
			line := left + strings.Repeat(" ", pad) + strings.Repeat(" ", tagW-wlen(tl.tag)) + tag
			body = append(body, line)
		}
		if len(c.unblocked) > 0 {
			head := "unblocks " + strconv.Itoa(len(c.unblocked))
			// Pre-render every unblocked dependent into its lines (ID line +
			// wrapped title + WP + verdict), then window them by line so the
			// section keeps a fixed height and scrolls with j/k.
			var all []string
			for _, e := range c.unblocked {
				all = append(all, effectLines(inner, e)...)
			}
			total := len(all)
			if total <= deleteLines {
				m.deleteScroll = 0
			}
			m.deleteScroll = min(m.deleteScroll, max(0, total-deleteLines))
			page := ""
			if total > deleteLines {
				page = fmt.Sprintf("%d/%d", m.deleteScroll/deleteLines+1,
					(total+deleteLines-1)/deleteLines)
			}
			body = append(body, "", sectionRule(head, page, inner), "")
			// Always render exactly deleteLines lines (blank-padded) so the
			// dialog height stays constant while scrolling and regardless of
			// how many dependents unblock.
			end := min(m.deleteScroll+deleteLines, total)
			for _, l := range all[m.deleteScroll:end] {
				body = append(body, l)
			}
			for range deleteLines - (end - m.deleteScroll) {
				body = append(body, "")
			}
		}
		keys := styles.DimS.Render("[esc] cancel    ") + styles.AccentS.Render("[y] delete")
		return boxWithKeys("delete "+c.id+"?", styles.Accent, body, keys, inner, w)
	case OverlayConfirm:
		node, ok := m.byID[m.confirmID]
		if !ok {
			return ""
		}
		v := node.Verification
		lines := []string{
			"",
			styles.FgS.Render(v.AgentName + " ran"),
			"  " + styles.BrightS.Render(node.Condition),
			styles.FgS.Render(v.AgentWhen+" and it ") + styles.S.Copy().Foreground(styles.Blocked).Render("failed") + ".",
			"",
			styles.DimS.Render("Marking this verified records your acceptance"),
			styles.DimS.Render("over that result."),
			"",
			styles.DimS.Render("[esc] cancel    ") + styles.AccentS.Render("[y] accept anyway"),
			"",
		}
		return box("accept over agent failure?", "blocked", lines, w)

	case OverlayStatus:
		var lines []string
		// current status of the target node, for the grey "(current)" marker
		current := ""
		if n, ok := m.current(); ok {
			current = string(m.ownerTask(n).Status)
		}
		// shortcut letter per status, in the status colour
		keyFor := map[store.Status]string{
			store.Ready:    "r",
			store.Blocked:  "b",
			store.Deferred: "x",
			store.Done:     "d",
		}
		// The footer line is the widest line and therefore sets the dialog
		// width. Pad every status row so its shortcut sits flush against the
		// right wall (with a small margin) without widening the dialog.
		footer := styles.DimS.Render("j/k move   r/b/x/d set   ret set   esc cancel")
		inner := wlen(stripANSI(footer))
		if inner > w-6 {
			inner = w - 6
		}
		for i, s := range store.AllStatuses {
			marker := "  "
			if i == m.statusIdx {
				marker = styles.AccentS.Render("▸ ")
			}
			name := string(s)
			if name == current {
				name += "  " + styles.DimS.Render("(current)")
			}
			left := marker + styles.Status(s).Render(name)
			key := styles.Status(s).Render(keyFor[s])
			pad := inner - 3 - wlen(stripANSI(left)) - wlen(stripANSI(key))
			if pad < 1 {
				pad = 1
			}
			lines = append(lines, left+strings.Repeat(" ", pad)+key)
		}
		lines = append(lines, "", footer, "")
		return box("set status", "", lines, w)

	case OverlayProjects:
		var lines []string
		lines = append(lines, "")
		for i, p := range m.projects {
			marker := "  "
			nameS := styles.FgS
			if i == m.projectIdx {
				marker = styles.AccentS.Render("▸ ")
				nameS = styles.BrightS
			}
			lines = append(lines, marker+nameS.Render(padTrunc(p.Name, 22))+" "+styles.DimS.Render(p.Description))
		}
		lines = append(lines, "", styles.DimS.Render("j/k move   ret open   esc cancel"), "")
		return box("projects", "", lines, w)

	case OverlayComment:
		lines := []string{"", m.input.View(), "", styles.DimS.Render("ret send   esc cancel"), ""}
		return box("leave a note", "", lines, w)

	case OverlayFinder:
		lines := []string{pad(m.input.View(), finderInner)}
		lines = append(lines, styles.DimS.Render(strings.Repeat("─", finderInner)))
		// The dialog is fixed-height: always render finderVisible result rows
		// (hint + padded empties when there are no/too few matches), each
		// padded to finderInner so the box width is constant too.
		rows := make([]string, finderVisible)
		if len(m.finderHits) == 0 {
			rows[0] = pad(styles.DimS.Render("  search tasks and steps — try \"rotate\""), finderInner)
		} else {
			// show a fixed-size window of hits; finderScroll tracks the first
			// visible row so the dialog keeps a constant height regardless of
			// how many results match.
			visStart := m.finderScroll
			if visStart > len(m.finderHits) {
				visStart = len(m.finderHits)
			}
			visEnd := visStart + finderVisible
			if visEnd > len(m.finderHits) {
				visEnd = len(m.finderHits)
			}
			for k, i := 0, visStart; i < visEnd; i, k = i+1, k+1 {
				n := m.finderHits[i]
				marker := "  "
				titleS := styles.FgS
				if i == m.finderIdx {
					marker = styles.AccentS.Render("▸ ")
					titleS = styles.BrightS
				}
				kind := "●"
				if n.Type == store.Step {
					kind = styles.StepGlyph(n.Status)
				}
				row := marker + styles.Status(n.Status).Render(kind) + " " +
					styles.DimS.Render(padTrunc(n.ID, 9)) + " " + titleS.Render(padTrunc(n.Title, finderInner-14))
				rows[k] = pad(row, finderInner)
			}
		}
		lines = append(lines, rows...)
		sc := ""
		if len(m.finderHits) > finderVisible {
			sc = "  " + styles.DimS.Render(fmt.Sprintf("(%d/%d)", m.finderIdx+1, len(m.finderHits)))
		}
		foot := "  ↑↓ move   ret open   esc close" + sc
		lines = append(lines, styles.DimS.Render(strings.Repeat("─", finderInner)),
			pad(styles.DimS.Render(foot), finderInner))
		return box("find", "", lines, w)
	}
	return ""
}

// unblockLines renders one unblocked dependent as a wrapped group: the first
// line carries the gutter + status glyph + node ID; the title follows on its
// own line(s), indented to align with the ⟨WP⟩ and status-verdict lines below
// (the node ID column), so a long title wraps instead of being cut off at the
// dialog edge.
func unblockLines(inner int, e effect) []string {
	glyph := styles.Status(e.node.Status).Render("●")
	if e.stuck {
		glyph = styles.S.Copy().Foreground(styles.Deferred).Render("◇")
	}
	first := styles.DimS.Render("└ ") + glyph + " " + styles.FgS.Render(e.node.ID)
	title := e.node.Title
	avail := inner - 4
	if wlen(title) <= avail {
		return []string{first, "    " + styles.FgS.Render(title)}
	}
	wrap := wrapPlain(title, avail)
	lines := []string{first}
	indent := strings.Repeat(" ", 4) // aligns under the node ID, like the WP/status lines
	for _, wl := range wrap {
		lines = append(lines, indent+styles.FgS.Render(wl))
	}
	return lines
}

// effectLines renders one effect (an unblocked or cascaded dependent) the way
// the delete confirm and cascade panels both show it: an ID line with the
// wrapped title on its own indented line(s), a cross-package WP line when the
// dependent lives in a different package, then the status verdict. Both panels
// share this so their unblocks sections stay visually identical.
func effectLines(inner int, e effect) []string {
	trans := styles.Status(e.from).Render(string(e.from)) +
		styles.DimS.Render(" → ") + styles.Status(e.to).Render(string(e.to))
	if e.stuck {
		trans = styles.DimS.Render("still blocked")
	}
	lines := unblockLines(inner, e)
	if e.crossWP != "" {
		hue := styles.Hues[wpHue(e.node.ParentID)]
		lines = append(lines, "    "+styles.S.Copy().Foreground(hue).Render(e.crossWP))
	}
	return append(lines, "    "+trans)
}
