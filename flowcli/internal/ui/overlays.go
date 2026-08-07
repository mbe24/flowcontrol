package ui

import (
	"fmt"
	"strings"

	lv2 "charm.land/lipgloss/v2"

	"flowcli/internal/store"
	"flowcli/internal/styles"
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
	// the header line must match the body width (inner+4: wall + space +
	// content + space + wall). `len` counts bytes and box glyphs are
	// multibyte, so compute dashes from the display width wlen(head); the
	// "+1" accounts for the ┐ corner.
	dashes := max(inner+4-wlen(head)-1, 0)
	b.WriteString(accent.Render(head+strings.Repeat("─", dashes)+"┐") + "\n")
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
				row := marker + styles.Status(n.Status).Render(kind)+" "+
					styles.DimS.Render(padTrunc(n.ID, 9))+" "+titleS.Render(padTrunc(n.Title, 30))
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
	case OverlayHelp:
		km := kmForScreen(m.screen)
		helpView := m.help.FullHelpView(km.FullHelp())
		lines := split(helpView)
		return box("key bindings", "", lines, w)
	}
	return ""
}

// split returns the individual lines of s (which may itself contain newlines),
// trimming blank trailing space so box() sizes itself to the content.
func split(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		out = append(out, line)
	}
	return out
}
