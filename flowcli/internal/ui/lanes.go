package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// laneSet picks how many lanes fit. Thresholds are derived from the card
// widths, not chosen independently: 4×22+gutters=98, 2×30+gutter=67.
func (m Model) laneSet() []store.Status {
	switch {
	case m.width >= FourLaneMin:
		return store.AllStatuses
	case m.width >= TwoLaneMin:
		return []store.Status{store.Ready, store.Blocked}
	default:
		return []store.Status{store.AllStatuses[min(m.lane, 3)]}
	}
}

// laneGutter returns the spacing between lanes for the chosen lane set.
func (m Model) laneGutter() int {
	switch len(m.laneSet()) {
	case 4:
		return 2
	default:
		return 3
	}
}

// laneWidths distributes the frame's full inner width across the lanes so the
// lane view fills the terminal edge-to-edge, exactly like every other view.
// Unlike the old fixed 22/30 per-lane caps, lanes here stretch to the
// available width (with the remainder spread over the last few lanes), so the
// right wall reaches the terminal edge instead of leaving a black margin.
func (m Model) laneWidths(lanes []store.Status, gutter, inner int) []int {
	n := len(lanes)
	usable := inner - (n-1)*gutter
	if usable < n {
		usable = n
	}
	base := usable / n
	rem := usable % n
	widths := make([]int, n)
	for i := 0; i < n; i++ {
		widths[i] = base
		if i >= n-rem {
			widths[i]++
		}
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	return widths
}

func (m Model) laneTasks(s store.Status) []store.Node {
	var out []store.Node
	for _, n := range m.nodes {
		if n.Type == store.Task && n.Status == s {
			out = append(out, n)
		}
	}
	return out
}

// wrapTo wraps to at most max lines, marking a cut with a hyphen so the
// truncation is visible rather than silent.
func wrapTo(s string, w, maxLines int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if len(line)+len(word)+1 > w {
			lines = append(lines, line)
			line = word
		} else if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := lines[maxLines-1]
		if len(last) > w-1 {
			last = last[:w-1]
		}
		lines[maxLines-1] = strings.TrimRight(last, " ") + "-"
	}
	for len(lines) < maxLines {
		lines = append(lines, "")
	}
	return lines
}

// cell hard-truncates to w display cells so no cell can ever push its
// neighbour. `rendered` is the coloured version of `plain`.
func cell(plain, rendered string, w int) string {
	if wlen(plain) > w {
		// hard-truncate the plain text to w cells, then trim a trailing partial
		// glyph so we never emit an odd half-width.
		r := []rune(stripANSI(plain))
		s := ""
		for _, c := range r {
			if wlen(s+string(c)) > w {
				break
			}
			s += string(c)
		}
		return s
	}
	return rendered + strings.Repeat(" ", w-wlen(plain))
}

// cardLine is one rendered row of a lane card: the plain (unstyled) text and
// the styled version of the same content.
type cardLine struct{ plain, rendered string }

// laneCard builds the visible rows of a single lane card: the header line
// (status mark + ID + step ratio + verdict glyph), the wrapped title lines,
// and the work-package name footer. Empty wrapped lines are dropped so the
// footer always sits directly under the last title line instead of leaving a
// stray blank before it (previously any one-line title put a gap before the
// work-package name).
func (m Model) laneCard(t store.Node, laneW int, sel bool) []cardLine {
	var col []cardLine
	done, total := m.stepRatio(t.ID)
	ratio := fmt.Sprintf("%d/%d", done, total)
	b := t.Verification.Badge()

	mark := " "
	idStyle := styles.FgS
	if sel {
		mark = "▸"
		idStyle = styles.AccentS
	}
	head := mark + t.ID
	tail := ratio + " " + b.Glyph
	gap := laneW - wlen(head) - wlen(tail)
	if gap < 1 {
		gap = 1
	}
	col = append(col, cardLine{
		head + strings.Repeat(" ", gap) + tail,
		styles.AccentS.Render(mark) + idStyle.Render(t.ID) + strings.Repeat(" ", gap) +
			styles.DimS.Render(ratio) + " " + styles.Status(b.Kind).Render(b.Glyph),
	})

	titleStyle := styles.FgS
	if sel {
		titleStyle = styles.BrightS
	}
	for _, l := range wrapTo(t.Title, laneW-1, 2) {
		if l == "" {
			continue
		}
		col = append(col, cardLine{" " + l, " " + titleStyle.Render(l)})
	}

	pkg := ""
	if p, ok := m.byID[t.ParentID]; ok {
		pkg = p.Title
	}
	// Keep the footnote inside the lane: reserve the annotation's width up
	// front and clip the WP name to what's left, so the coloured string fits
	// and never falls through to cell()'s colour-dropping plain path.
	ann := ""
	if laneW >= 30 && len(m.blockers[t.ID]) > 0 {
		cand := " ←" + m.blockers[t.ID][0]
		if wlen(" "+pkg)+wlen(cand) <= laneW {
			ann = cand
		}
	}
	avail := laneW - 1 - wlen(ann)
	if avail < 1 {
		avail = 1
	}
	clipped := clipRunes(pkg, avail)
	hue := styles.Hues[wpHue(t.ParentID)]
	foot := " " + clipped
	footR := " " + lipgloss.NewStyle().Foreground(hue).Render(clipped)
	if ann != "" {
		foot += ann
		footR += styles.DimS.Render(ann)
	}
	return append(col, cardLine{foot, footR})
}

// clipRunes cuts s to at most w display cells, trimming a trailing partial
// glyph so no half-width ever reaches the terminal.
func clipRunes(s string, w int) string {
	out := ""
	for _, c := range []rune(stripANSI(s)) {
		if wlen(out+string(c)) > w {
			break
		}
		out += string(c)
	}
	return out
}

func (m Model) viewLanes(w, h int) string {
	lanes := m.laneSet()
	gutter := m.laneGutter()
	inner := w - 4
	widths := m.laneWidths(lanes, gutter, inner)

	built := make([][]cardLine, len(lanes))

	for li, st := range lanes {
		laneW := widths[li]
		tasks := m.laneTasks(st)
		cur := m.laneCursor[li]
		var col []cardLine
		for ti, t := range tasks {
			if ti > 0 {
				col = append(col, cardLine{"", ""})
			}
			sel := li == m.lane && ti == cur
			col = append(col, m.laneCard(t, laneW, sel)...)
		}
		col = append(col, cardLine{"", ""})
		more := fmt.Sprintf(" +%d more", max(0, len(tasks)-3))
		col = append(col, cardLine{more, styles.DimS.Render(more)})
		built[li] = col
	}

	var body []string

	// headers
	var hp, hr strings.Builder
	for i, st := range lanes {
		if i > 0 {
			hp.WriteString(strings.Repeat(" ", gutter))
			hr.WriteString(strings.Repeat(" ", gutter))
		}
		label := fmt.Sprintf("● %s %d", st, len(m.laneTasks(st)))
		hp.WriteString(padTrunc(label, widths[i]))
		hr.WriteString(cell(label, styles.Status(st).Render(label), widths[i]))
	}
	body = append(body, hr.String())

	var rp, rr strings.Builder
	for i, st := range lanes {
		if i > 0 {
			rp.WriteString(strings.Repeat(" ", gutter))
			rr.WriteString(strings.Repeat(" ", gutter))
		}
		// Rule spans the full lane width so the colored line reaches past the
		// condition check-mark in the card header and aligns with the lane's
		// right border, matching the left border.
		rule := strings.Repeat("─", widths[i])
		rr.WriteString(cell(rule, styles.Status(st).Render(rule), widths[i]))
	}
	body = append(body, rr.String())

	// Each lane scrolls independently: compute a per-lane vertical offset so
	// that lane's active card (ID + title + work-package) stays fully visible,
	// then render enough rows to fill the grid viewport, sampling each lane at
	// its own offset. frame() reserves height-4 body rows, 2 of which go to
	// the fixed header + rule above, so the grid gets the rest.
	gridRoom := h - 6 // header(2) + frame padding(4)
	if len(lanes) == 1 {
		gridRoom-- // one-lane mode shows a dot row after the grid
	}
	if gridRoom < 1 {
		gridRoom = 1
	}

	scrolls := make([]int, len(lanes))
	for li := range lanes {
		st := lanes[li]
		laneW := widths[li]
		tasks := m.laneTasks(st)
		cur := m.laneCursor[li]
		// top row of the active card within this lane's column
		top := 0
		for ti := 0; ti < cur && ti < len(tasks); ti++ {
			top += len(m.laneCard(tasks[ti], laneW, false)) + 1
		}
		if cur < len(tasks) {
			h := len(m.laneCard(tasks[cur], laneW, li == m.lane))
			bottom := top + h
			colH := len(built[li])
			scrolls[li] = bottom - gridRoom
			if scrolls[li] < 0 {
				scrolls[li] = 0
			}
			if scrolls[li] > max(0, colH-gridRoom) {
				scrolls[li] = max(0, colH-gridRoom)
			}
		}
	}

	for r := 0; r < gridRoom; r++ {
		var line strings.Builder
		for i := range lanes {
			if i > 0 {
				line.WriteString(strings.Repeat(" ", gutter))
			}
			var cl cardLine
			if idx := scrolls[i] + r; idx >= 0 && idx < len(built[i]) {
				cl = built[i][idx]
			}
			line.WriteString(cell(cl.plain, cl.rendered, widths[i]))
		}
		body = append(body, line.String())
	}

	keys := m.statusLine(lanesKeys(), m.screen, inner)
	if len(lanes) == 1 {
		keys = m.statusLine(lanesKeys(), m.screen, inner)
		dots := make([]string, 4)
		for i := range dots {
			if i == m.lane {
				dots[i] = styles.AccentS.Render("●")
			} else {
				dots[i] = styles.DimS.Render("○")
			}
		}
		body = append(body, "   "+strings.Join(dots, " "))
	}

	return frame("flowcli ─ lanes", body, keys, inner, h)
}

func padTrunc(s string, w int) string {
	if wlen(s) > w {
		// cell-accurate truncation
		out := ""
		for _, c := range []rune(stripANSI(s)) {
			if wlen(out+string(c)) > w {
				break
			}
			out += string(c)
		}
		return out
	}
	return s + strings.Repeat(" ", w-wlen(s))
}
