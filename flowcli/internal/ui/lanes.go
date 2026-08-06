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

func (m Model) laneWidth() (laneW, gutter int) {
	switch len(m.laneSet()) {
	case 4:
		return 22, 2
	case 2:
		return 30, 3
	default:
		return min(m.width-6, 34), 0
	}
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

// cell hard-truncates to w so no cell can ever push its neighbour.
func cell(plain, rendered string, w int) string {
	if len(plain) > w {
		return plain[:w]
	}
	return rendered + strings.Repeat(" ", w-len(plain))
}

func (m Model) viewLanes(w, h int) string {
	lanes := m.laneSet()
	laneW, gutter := m.laneWidth()
	inner := len(lanes)*laneW + (len(lanes)-1)*gutter

	type cardLine struct{ plain, rendered string }
	built := make([][]cardLine, len(lanes))

	for li, st := range lanes {
		tasks := m.laneTasks(st)
		cur := m.laneCursor[li]
		var col []cardLine
		for ti, t := range tasks {
			if ti > 0 {
				col = append(col, cardLine{"", ""})
			}
			sel := li == m.lane && ti == cur
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
			gap := laneW - len(head) - len(tail)
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
				col = append(col, cardLine{" " + l, " " + titleStyle.Render(l)})
			}

			pkg := ""
			if p, ok := m.byID[t.ParentID]; ok {
				pkg = p.Title
			}
			hue := styles.Hues[m.hueOf(t.ParentID)%len(styles.Hues)]
			foot := " " + pkg
			footR := " " + lipgloss.NewStyle().Foreground(hue).Render(pkg)
			// the ←blocker annotation is the first thing dropped when narrow
			if laneW >= 30 && len(m.blockers[t.ID]) > 0 {
				ann := " ←" + m.blockers[t.ID][0]
				if len(foot)+len(ann) <= laneW {
					foot += ann
					footR += styles.DimS.Render(ann)
				}
			}
			col = append(col, cardLine{foot, footR})
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
		hp.WriteString(padTrunc(label, laneW))
		hr.WriteString(cell(label, styles.Status(st).Render(label), laneW))
	}
	body = append(body, hr.String())

	var rp, rr strings.Builder
	for i, st := range lanes {
		if i > 0 {
			rp.WriteString(strings.Repeat(" ", gutter))
			rr.WriteString(strings.Repeat(" ", gutter))
		}
		rule := strings.Repeat("─", min(laneW-2, len(st)+4))
		rr.WriteString(cell(rule, styles.Status(st).Render(rule), laneW))
	}
	body = append(body, rr.String())

	depth := 0
	for _, c := range built {
		if len(c) > depth {
			depth = len(c)
		}
	}
	for r := 0; r < depth; r++ {
		var line strings.Builder
		for i := range lanes {
			if i > 0 {
				line.WriteString(strings.Repeat(" ", gutter))
			}
			var cl cardLine
			if r < len(built[i]) {
				cl = built[i][r]
			}
			line.WriteString(cell(cl.plain, cl.rendered, laneW))
		}
		body = append(body, line.String())
	}

	keys := key("h/l") + " lane  " + key("j/k") + " card  " + key("ret") + " detail  " +
		key("s") + " status  " + key("1") + " tree  " + key("3") + " chain"
	if len(lanes) == 1 {
		keys = key("tab") + " lane  " + key("j/k") + " card  " + key("ret") + " detail  " + key("s") + " status"
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
	if len(s) > w {
		return s[:w]
	}
	return s + strings.Repeat(" ", w-len(s))
}
