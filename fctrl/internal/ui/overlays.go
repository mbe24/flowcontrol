package ui

import (
	"fmt"
	"strings"

	"fctrl/internal/store"
	"fctrl/internal/styles"
)

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
	head := "┌─ " + title + " "
	b.WriteString(accent.Render(head+strings.Repeat("─", max(inner+2-len(head)-1+2, 0))+"┐") + "\n")
	wall := styles.DimS.Render("│")
	for _, l := range lines {
		fill := inner - len([]rune(stripANSI(l)))
		if fill < 0 {
			fill = 0
		}
		b.WriteString(wall + " " + l + strings.Repeat(" ", fill) + " " + wall + "\n")
	}
	b.WriteString(accent.Render("└" + strings.Repeat("─", inner+2) + "┘"))
	return b.String()
}

func (m Model) viewOverlay(w int) string {
	switch m.overlay {
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
		lines = append(lines, "")
		for i, s := range store.AllStatuses {
			marker := "  "
			if i == m.statusIdx {
				marker = styles.AccentS.Render("▸ ")
			}
			lines = append(lines, marker+styles.Status(s).Render(string(s)))
		}
		lines = append(lines, "", styles.DimS.Render("j/k move   ret set   esc cancel"), "")
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
		lines := []string{m.input.View(), styles.DimS.Render(strings.Repeat("─", 46))}
		if len(m.finderHits) == 0 {
			lines = append(lines, styles.DimS.Render("  search tasks and steps — try \"rotate\""))
		}
		for i, n := range m.finderHits {
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
			lines = append(lines, marker+styles.Status(n.Status).Render(kind)+" "+
				styles.DimS.Render(padTrunc(n.ID, 9))+" "+titleS.Render(padTrunc(n.Title, 42)))
		}
		lines = append(lines, styles.DimS.Render(strings.Repeat("─", 46)),
			styles.DimS.Render(fmt.Sprintf("  %d results   ↑↓ move   ret open   esc close", len(m.finderHits))))
		return box("find", "", lines, w)
	}
	return ""
}
