package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowcontrol/fctrl/internal/store"
	"flowcontrol/fctrl/internal/styles"
)

func (m Model) View() string {
	if m.err != nil {
		return styles.StatusStyle(store.StatusBlocked).Render("error: "+m.err.Error()) + "\n"
	}
	switch m.screen {
	case screenProjects:
		return m.viewProjects()
	case screenHelp:
		return m.viewHelp()
	}
	return m.viewBrowser()
}

// ── chrome ────────────────────────────────────────────────────────────────

func pad(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clamp(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func clamp(s string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func rule(width int) string {
	if width < 1 {
		return ""
	}
	return styles.RuleStyle.Render(strings.Repeat("─", width))
}

func (m Model) header(width int) string {
	done, ready, blocked, total := m.projectCounts()
	left := styles.Title.Render("fctrl") + styles.Faint.Render(" · ") + styles.Head.Render(m.projectName())
	right := styles.Bar(done, ready, blocked, total, 20) + "  " +
		styles.Soft.Render(pct(done, total)) + styles.Faint.Render("  "+itoa(done)+"/"+itoa(total))
	return pad(left, right, width)
}

func (m Model) statusLine(width int) string {
	keys := "j/k move · space fold · ↵ detail · / find · s status · d done · v verify · ? keys"
	if m.focusDetail {
		keys = "tab tree · s status · d done · v verify · u undo · esc back · ? keys"
	}
	right := ""
	switch {
	case m.verifying:
		right = m.spin.View() + " running condition"
	case m.flash != "":
		right = m.flash
	default:
		if n, ok := m.detailNode(); ok {
			g, c := styles.VerifyGlyph(n.LastResult)
			run := n.LastRun
			if run == "" {
				run = "never"
			}
			right = lipgloss.NewStyle().Foreground(c).Render(g) + styles.Faint.Render(" verify "+run)
		}
	}
	return pad(styles.Faint.Render(keys), styles.Soft.Render(right), width)
}

// ── browser ───────────────────────────────────────────────────────────────

func (m Model) viewBrowser() string {
	bodyH := max(m.h-4, 6)
	narrow := m.w < 100

	var body string
	switch {
	case m.mode == modeFinder:
		body = lipgloss.Place(m.w, bodyH, lipgloss.Center, lipgloss.Center, m.viewFinder(min(m.w-8, 78)))
	case narrow && m.focusDetail:
		body = m.detailPane(m.w, bodyH)
	case narrow:
		body = m.treePane(m.w, bodyH)
	default:
		treeW := m.w * 58 / 100
		detailW := m.w - treeW - 1
		sep := styles.RuleStyle.Render(strings.Repeat("│\n", max(bodyH-1, 1)) + "│")
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.treePane(treeW, bodyH), sep, m.detailPane(detailW, bodyH))
	}

	return strings.Join([]string{
		m.header(m.w),
		rule(m.w),
		body,
		rule(m.w),
		m.statusLine(m.w),
	}, "\n")
}

func (m Model) treePane(width, height int) string {
	lines := []string{}
	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	for i := start; i < len(m.rows) && len(lines) < height; i++ {
		lines = append(lines, m.rowLine(m.rows[i], width, i == m.cursor && !m.focusDetail))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", 0))
	}

	// The status picker is an inline overlay: it replaces the rows under the
	// cursor rather than floating.
	if m.mode == modeStatus || m.mode == modeConfirm {
		box := m.viewStatusOverlay(min(width-2, 52))
		bl := strings.Split(box, "\n")
		at := min(m.cursor-start+1, max(height-len(bl), 0))
		for i, l := range bl {
			if at+i < len(lines) {
				lines[at+i] = clamp(" "+l, width)
			}
		}
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) rowLine(r row, width int, selected bool) string {
	var left, right string

	switch r.kind {
	case rowWP:
		caret := "▸"
		if m.expanded[r.node.ID] {
			caret = "▾"
		}
		left = styles.Soft.Render(caret) + " " + styles.Head.Render(shorten(r.node.Title, max(width-38, 12)))
		done, ready, blocked, total := m.wpCounts(r.node.ID)
		state := styles.Faint.Render(string(r.node.State))
		if r.node.State == store.StateActive {
			state = styles.StatusStyle(store.StatusReady).Render("ACTIVE")
		}
		right = state + "  " + styles.Bar(done, ready, blocked, total, 10) + " " + styles.Faint.Render(pct(done, total))

	case rowTask:
		caret := " "
		if r.hasKids {
			caret = "▸"
			if m.expanded[r.node.ID] {
				caret = "▾"
			}
		}
		dot := styles.StatusStyle(r.node.Status).Render(styles.Glyph(r.node.Status))
		title := styles.Body.Render(shorten(r.node.Title, max(width-44, 12)))
		if r.node.Status == store.StatusDone {
			title = styles.Soft.Render(shorten(r.node.Title, max(width-44, 12)))
		}
		left = "  " + styles.Faint.Render(caret) + " " + dot + " " + styles.Faint.Render(r.node.ID) + "  " + title

		bl := m.blockers[r.node.ID]
		if len(bl) > 0 {
			label := "← " + bl[0]
			if len(bl) > 1 {
				label += " +" + itoa(len(bl)-1)
			}
			right = styles.StatusStyle(store.StatusBlocked).Render(label) + "  "
		}
		g, c := styles.VerifyGlyph(r.node.LastResult)
		right += lipgloss.NewStyle().Foreground(c).Render(g) + " " +
			styles.Faint.Render(m.stepRatio(r.node.ID)) + "  " +
			styles.StatusStyle(r.node.Status).Render(string(r.node.Status))

	case rowStep:
		branch := "├─"
		if m.isLastStep(r.node) {
			branch = "╰─"
		}
		g := styles.StatusStyle(r.node.Status).Render(styles.StepGlyph(r.node.Status))
		left = "      " + styles.Faint.Render(branch) + " " + g + " " +
			styles.Soft.Render(shorten(r.node.Title, max(width-32, 10)))
		if r.node.Condition != "" {
			right = styles.Faint.Render(shorten(r.node.Condition, 18))
		}

	case rowArchived:
		left = styles.Faint.Render("▸ " + r.label)
		right = styles.Faint.Render("a to unhide")
	}

	if selected {
		marked := styles.Title.Render("❯") + strings.TrimPrefix(left, " ")
		return styles.Cursor.Width(width).Render(clamp(pad(marked, right, width-1), width-1))
	}
	return clamp(pad(left, right, width), width)
}

func (m Model) detailPane(width, height int) string {
	n, ok := m.detailNode()
	if !ok {
		return lipgloss.NewStyle().Width(width).Height(height).Render(
			styles.Faint.Render("  nothing selected"))
	}
	inner := max(width-4, 20)
	out := []string{}
	add := func(s string) { out = append(out, "  "+clamp(s, inner)) }

	add(pad(styles.Faint.Render(n.ID+" · "+string(n.Type)),
		styles.StatusStyle(n.Status).Render(string(n.Status)), inner))
	add("")
	for _, l := range wrap(n.Title, inner) {
		add(styles.Head.Render(l))
	}
	if n.Description != "" {
		add("")
		for _, l := range wrap(n.Description, inner) {
			add(styles.Soft.Render(l))
		}
	}

	add("")
	add(styles.Label.Render("CONDITION"))
	if n.Condition == "" {
		add(styles.Faint.Render("none set · e to add"))
	} else {
		add(styles.Body.Render(n.Condition))
		g, c := styles.VerifyGlyph(n.LastResult)
		run := n.LastRun
		if run == "" {
			run = "never run"
		}
		if m.verifying {
			add(m.spin.View() + styles.Soft.Render(" running…"))
		} else {
			add(pad(lipgloss.NewStyle().Foreground(c).Render(g)+styles.Soft.Render(" "+run),
				styles.Title.Render("v re-verify"), inner))
		}
	}

	steps := m.stepsOf(n.ID)
	if len(steps) > 0 {
		add("")
		add(pad(styles.Label.Render("STEPS"), styles.Faint.Render(m.stepRatio(n.ID)), inner))
		for _, s := range steps {
			g := styles.StatusStyle(s.Status).Render(styles.StepGlyph(s.Status))
			title := styles.Soft.Render(shorten(s.Title, inner-6))
			if s.Status == store.StatusDone {
				title = styles.Faint.Render(shorten(s.Title, inner-6))
			}
			add(" " + g + " " + title)
		}
	}

	bl, bk := m.blockers[n.ID], m.blocks[n.ID]
	if len(bl)+len(bk) > 0 {
		add("")
		add(styles.Label.Render("DEPENDENCIES"))
		for _, id := range bl {
			add(m.depLine("blocked by", id, n, inner))
		}
		for _, id := range bk {
			add(m.depLine("blocks    ", id, n, inner))
		}
	}

	for len(out) < height {
		out = append(out, "")
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(out[:height], "\n"))
}

func (m Model) depLine(dir, id string, self store.Node, inner int) string {
	other, ok := m.byID[id]
	title := "(outside this project)"
	st := store.StatusBlocked
	if ok {
		title = other.Title
		st = other.Status
	}
	note := ""
	if ok && other.Type == store.TypeTask && self.Type == store.TypeTask &&
		other.ParentID != self.ParentID {
		note = "cross-pkg"
	}
	left := styles.Faint.Render(dir) + " " +
		styles.StatusStyle(st).Render(styles.Glyph(st)) + " " +
		styles.Faint.Render(id) + " " +
		styles.Soft.Render(shorten(title, max(inner-30, 8)))
	return pad(left, styles.Faint.Render(note), inner)
}

// ── counts ────────────────────────────────────────────────────────────────

func (m Model) stepsOf(taskID string) []store.Node {
	out := []store.Node{}
	for _, n := range m.nodes {
		if n.ParentID == taskID && n.Type == store.TypeStep {
			out = append(out, n)
		}
	}
	return out
}

func (m Model) isLastStep(s store.Node) bool {
	steps := m.stepsOf(s.ParentID)
	return len(steps) > 0 && steps[len(steps)-1].ID == s.ID
}

func (m Model) stepRatio(taskID string) string {
	steps := m.stepsOf(taskID)
	if len(steps) == 0 {
		return "–"
	}
	done := 0
	for _, s := range steps {
		if s.Status == store.StatusDone {
			done++
		}
	}
	return itoa(done) + "/" + itoa(len(steps))
}

func (m Model) wpCounts(wpID string) (done, ready, blocked, total int) {
	for _, n := range m.nodes {
		if n.Type != store.TypeTask || n.ParentID != wpID {
			continue
		}
		total++
		switch n.Status {
		case store.StatusDone:
			done++
		case store.StatusReady:
			ready++
		case store.StatusBlocked:
			blocked++
		}
	}
	return
}

func (m Model) projectCounts() (done, ready, blocked, total int) {
	for _, n := range m.nodes {
		if n.Type == store.TypeWorkPackage {
			continue
		}
		total++
		switch n.Status {
		case store.StatusDone:
			done++
		case store.StatusReady:
			ready++
		case store.StatusBlocked:
			blocked++
		}
	}
	return
}

func pct(done, total int) string {
	if total == 0 {
		return "0%"
	}
	return itoa(done*100/total) + "%"
}

func wrap(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	words := strings.Fields(s)
	lines := []string{}
	cur := ""
	for _, w := range words {
		if cur == "" {
			cur = w
			continue
		}
		if len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
