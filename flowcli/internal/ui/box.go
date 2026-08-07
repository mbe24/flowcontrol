package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowcli/internal/styles"
)

// boxWithKeys draws a fixed-width dialog: a titled top rule, body rows padded
// to `inner`, a divider, and a key line. It is the same shape as frame() but
// sized to its content rather than the terminal, so overlay() can centre it.
//
// This is the one primitive every new dialog in the create / cascade / delete
// family builds on.
func boxWithKeys(title string, accent lipgloss.Color, body []string, keys string, inner, termW int) string {
	if inner > termW-6 {
		inner = termW - 6
	}
	if inner < 20 {
		inner = 20
	}
	as := styles.S.Copy().Foreground(accent)
	wall := styles.DimS.Render("│")

	var b strings.Builder

	head := "╭─ " + title + " "
	dashes := inner + 2 - wlen(head) + 1
	if dashes < 0 {
		dashes = 0
	}
	b.WriteString(as.Render(head+strings.Repeat("─", dashes)+"╮") + "\n")

	for _, line := range body {
		fill := inner - wlen(stripANSI(line))
		if fill < 0 {
			fill = 0
			line = truncANSI(line, inner)
		}
		b.WriteString(wall + " " + line + strings.Repeat(" ", fill) + " " + wall + "\n")
	}

	if keys != "" {
		b.WriteString(styles.DimS.Render("├"+strings.Repeat("─", inner+2)+"┤") + "\n")
		fill := inner - wlen(stripANSI(keys))
		if fill < 0 {
			fill = 0
		}
		b.WriteString(wall + " " + keys + strings.Repeat(" ", fill) + " " + wall + "\n")
	}

	b.WriteString(as.Render("╰" + strings.Repeat("─", inner+2) + "╯"))
	return b.String()
}

// truncANSI cuts a styled string to w display cells, keeping escape sequences
// intact so colour never bleeds past the border.
func truncANSI(s string, w int) string {
	var out strings.Builder
	cells, inEsc := 0, false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			out.WriteRune(r)
			continue
		}
		if inEsc {
			out.WriteRune(r)
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if cells >= w {
			break
		}
		out.WriteRune(r)
		cells++
	}
	out.WriteString("\x1b[0m")
	return out.String()
}
