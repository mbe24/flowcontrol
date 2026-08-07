package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// ScreenLanding is the first thing flowcli shows: pick a project or make one.
// It is a screen rather than an overlay because there is nothing behind it to
// keep visible — on first run there is no project loaded at all.

// landingState holds the cursor for the project list. The list is small and
// already in memory, so this stays a plain index rather than a bubbles list;
// swap in bubbles/v2/list when project counts justify filtering and paging.
type landingState struct {
	cursor int
	// counts caches per-project done/total so the rows can show progress
	// without loading every project's nodes.
	counts map[string][2]int
}

func (m Model) visibleProjects() []store.Project {
	var out []store.Project
	for _, p := range m.projects {
		if !p.Archived {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) updateLanding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ps := m.visibleProjects()
	switch msg.String() {
	case "j", "down":
		m.landing.cursor = min(m.landing.cursor+1, len(ps)) // len == the "new" row
	case "k", "up":
		m.landing.cursor = max(m.landing.cursor-1, 0)
	case "n":
		m.openCreate(createProject, "", "", true)
		return m, nil
	case "enter":
		if m.landing.cursor >= len(ps) {
			m.openCreate(createProject, "", "", true)
			return m, nil
		}
		if len(ps) == 0 {
			return m, nil
		}
		m.projectID = ps[m.landing.cursor].ID
		m.screen = ScreenTree
		m.cursor, m.chainCursor, m.selectedID = 0, 0, ""
		return m, m.load
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) viewLanding(w, h int) string {
	ps := m.visibleProjects()
	// Full width: the landing is a screen (not a dialog), so it spans the
	// terminal like the tree/lanes/chain views rather than capping at 72.
	inner := w - 4

	var body []string
	body = append(body, "")

	for i, p := range ps {
		sel := i == m.landing.cursor
		hue := styles.Hues[i%len(styles.Hues)]

		mark := "  "
		nameStyle := styles.FgS
		if sel {
			mark = styles.AccentS.Render("❯ ")
			nameStyle = styles.BrightS
		}

		done, total := 0, 0
		if c, ok := m.landing.counts[p.ID]; ok {
			done, total = c[0], c[1]
		}
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}

		ratio := fmt.Sprintf("%d/%d", done, total)
		pctS := fmt.Sprintf("%d%%", pct)
		// ratio dim, percent in the project's hue (as before), and the pair
		// placed around the horizontal centre of the line rather than glued to
		// the name or pushed to the far edge.
		tail := "  " + styles.DimS.Render(ratio) + "  " +
			styles.S.Copy().Foreground(hue).Render(pctS)
		col := inner / 2 // start the ratio/percent column at centre width
		nameW := col - wlen(mark)
		if nameW < 0 {
			nameW = 0
		}
		name := padTrunc(p.Name, nameW)

		line := mark + nameStyle.Render(name) + tail
		body = append(body, line)

		// every project gets a fixed description line (and a blank line after)
		// so the list height is stable as the cursor moves.
		body = append(body, "  "+styles.DimS.Render(padTrunc(p.Description, inner-2)))
		body = append(body, "")
	}

	if len(ps) == 0 {
		body = append(body,
			styles.DimS.Render("  No projects yet."),
			"")
	}

	// The create row is part of the list, so ❯ can land on it and ↵ works
	// without a separate affordance.
	newSel := m.landing.cursor >= len(ps)
	newMark := "  "
	newStyle := styles.DimS
	if newSel {
		newMark = styles.AccentS.Render("❯ ")
		newStyle = styles.AccentS
	}
	body = append(body, newMark+newStyle.Render("+ new project"))

	keys := styles.AccentS.Render("j/k") + styles.DimS.Render(" move  ") +
		styles.AccentS.Render("↵") + styles.DimS.Render(" open  ") +
		styles.AccentS.Render("n") + styles.DimS.Render(" new project  ") +
		styles.AccentS.Render("q") + styles.DimS.Render(" quit")

	return frame("flowcli ─ select a project", body, keys, inner, h)
}

// sectionRule draws "├─ label ──── right ─┤" at the given content width.
func sectionRule(label, right string, width int) string {
	left := "─ " + label + " "
	tail := ""
	if right != "" {
		tail = " " + right + " ─"
	}
	fill := width - wlen(left) - wlen(tail)
	if fill < 0 {
		fill = 0
	}
	return styles.DimS.Render(left + strings.Repeat("─", fill) + tail)
}
