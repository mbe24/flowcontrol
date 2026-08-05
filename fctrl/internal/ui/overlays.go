package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowcontrol/fctrl/internal/store"
	"flowcontrol/fctrl/internal/styles"
)

// viewFinder is the fuzzy finder. In the real client it dims the tree behind
// it; here it is centred over the body area.
func (m Model) viewFinder(width int) string {
	inner := max(width-4, 24)
	lines := []string{
		pad(m.finder.View(), styles.Faint.Render(itoa(len(m.results))+"/"+itoa(len(m.nodes))), inner),
		rule(inner),
	}
	if len(m.results) == 0 {
		lines = append(lines, styles.Faint.Render("type to search tasks, steps and commands"))
	}
	for i, r := range m.results {
		glyph := styles.StatusStyle(r.node.Status).Render(styles.Glyph(r.node.Status))
		switch r.kind {
		case "step":
			glyph = styles.StatusStyle(r.node.Status).Render(styles.StepGlyph(r.node.Status))
		case "cmd":
			glyph = styles.Soft.Render("⌘")
		}
		left := " " + glyph + " " + styles.Faint.Render(padRight(r.id, 9)) + " " +
			styles.Body.Render(shorten(r.title, max(inner-28, 10)))
		line := clamp(pad(left, styles.Faint.Render(r.hint), inner), inner)
		if i == m.fCursor {
			line = styles.Cursor.Width(inner).Render(clamp(pad(styles.Title.Render("❯")+left, styles.Faint.Render(r.hint), inner-1), inner-1))
		}
		lines = append(lines, line)
	}
	lines = append(lines, rule(inner))
	lines = append(lines, pad(styles.Faint.Render("↵ jump · ⌥↵ set status · ⇥ scope to package"), styles.Faint.Render("esc"), inner))
	return styles.FocusBox.Width(width).Render(strings.Join(lines, "\n"))
}

// viewStatusOverlay is the inline status picker and, once DONE is chosen, the
// confirm bar that lists what the core will re-evaluate.
func (m Model) viewStatusOverlay(width int) string {
	n, ok := m.detailNode()
	if !ok {
		return ""
	}
	inner := max(width-4, 20)

	if m.mode == modeConfirm {
		lines := []string{
			styles.Soft.Render("marking ") + styles.Head.Render(n.ID) + styles.Soft.Render(" as ") +
				styles.StatusStyle(store.StatusDone).Render("DONE"),
		}
		open := 0
		for _, s := range m.stepsOf(n.ID) {
			if s.Status != store.StatusDone {
				open++
			}
		}
		if open > 0 {
			lines = append(lines, styles.Faint.Render(plural(open, "step", "steps")+" still open → close them too? ")+styles.Body.Render("[y/N]"))
		}
		dependents := m.blocks[n.ID]
		if len(dependents) > 0 {
			lines = append(lines, "", styles.Label.Render("THE CORE WILL RE-EVALUATE"))
			for _, id := range dependents {
				d, ok := m.byID[id]
				title := id
				if ok {
					title = d.ID + " " + shorten(d.Title, max(inner-16, 8))
				}
				lines = append(lines, styles.StatusStyle(store.StatusReady).Render(" + ")+styles.Soft.Render(title))
			}
			lines = append(lines, styles.Faint.Render(" cascade runs in the engine — this prototype writes one node"))
		}
		lines = append(lines, rule(inner), styles.StatusStyle(store.StatusReady).Render("↵ confirm")+styles.Faint.Render(" · esc cancel · u undo after"))
		return styles.OKBox.Width(width).Render(strings.Join(lines, "\n"))
	}

	lines := []string{styles.Faint.Render("set status → " + n.ID)}
	shortcuts := map[store.Status]string{
		store.StatusReady:    "r",
		store.StatusBlocked:  "b",
		store.StatusDeferred: "x",
		store.StatusDone:     "d",
	}
	for i, s := range store.AllStatuses {
		note := ""
		if s == n.Status {
			note = "current"
		} else if s == store.StatusBlocked {
			note = "usually derived"
		}
		left := " " + styles.Faint.Render(shortcuts[s]) + " " + styles.StatusStyle(s).Render(string(s))
		line := clamp(pad(left, styles.Faint.Render(note), inner), inner)
		if i == m.statusCursor {
			line = styles.Cursor.Width(inner).Render(clamp(pad(styles.Title.Render("❯")+left, styles.Faint.Render(note), inner-1), inner-1))
		}
		lines = append(lines, line)
	}
	lines = append(lines, styles.Faint.Render("↵ apply · esc cancel"))
	return styles.FocusBox.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) viewProjects() string {
	width := min(m.w-4, 72)
	inner := max(width-4, 24)
	lines := []string{
		styles.Title.Render("fctrl") + styles.Faint.Render(" — select a project"),
		rule(inner),
	}
	for i, p := range m.projects {
		left := "  " + styles.Body.Render(p.Name)
		if i == m.projCursor {
			left = styles.Title.Render("❯ ") + styles.Head.Render(p.Name)
		}
		line := clamp(pad(left, styles.Faint.Render(shorten(p.Description, max(inner-28, 8))), inner), inner)
		if i == m.projCursor {
			line = styles.Cursor.Width(inner).Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines,
		rule(inner),
		pad(styles.Faint.Render("  + new project"), styles.Faint.Render("n"), inner),
		"",
		styles.Faint.Render("↑↓ move · ↵ open · n new · q quit"),
	)
	box := styles.Box.Width(width).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.w, max(m.h-1, 8), lipgloss.Center, lipgloss.Center, box)
}

func (m Model) viewHelp() string {
	col := func(title string, rows [][2]string, c lipgloss.Color) string {
		out := []string{lipgloss.NewStyle().Foreground(c).Render(title), ""}
		for _, r := range rows {
			out = append(out, lipgloss.NewStyle().Foreground(c).Width(10).Render(r[0])+styles.Soft.Render(r[1]))
		}
		return strings.Join(out, "\n")
	}
	move := col("MOVE", [][2]string{
		{"j / k", "next / previous row"},
		{"g / G", "top / bottom"},
		{"space", "fold / unfold"},
		{"tab", "switch pane"},
		{"↵ / esc", "open / back"},
	}, styles.Focus)
	act := col("ACT", [][2]string{
		{"s", "set status"},
		{"d", "mark done"},
		{"v", "run condition"},
		{"u", "undo last change"},
	}, styles.Ready)
	find := col("FIND & SCOPE", [][2]string{
		{"/ ctrl-p", "fuzzy finder"},
		{"a", "show done packages"},
		{"p", "project picker"},
		{"?", "this screen"},
		{"q", "quit"},
	}, styles.Blocked)

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(30).Render(move),
		lipgloss.NewStyle().Width(30).Render(act),
		lipgloss.NewStyle().Width(30).Render(find))

	box := styles.Box.Render(strings.Join([]string{
		styles.Head.Render("Keymap"), "", body, "",
		styles.Faint.Render("Arrow keys alias every motion. ? or esc to close."),
	}, "\n"))
	return lipgloss.Place(m.w, max(m.h-1, 8), lipgloss.Center, lipgloss.Center, box)
}

func padRight(s string, n int) string {
	for len([]rune(s)) < n {
		s += " "
	}
	return s
}
