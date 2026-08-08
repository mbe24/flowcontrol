// Package styles holds the one palette shared by every screen. The colours
// match the web app's tokens so the two front doors look related.
package styles

import (
	"github.com/charmbracelet/lipgloss"

	"flowcli/internal/store"
)

var (
	Ready    = lipgloss.Color("#3ecf8e")
	Blocked  = lipgloss.Color("#ef6a5a")
	Deferred = lipgloss.Color("#7c8394")
	Done     = lipgloss.Color("#4a90d9")
	Accent   = lipgloss.Color("#5ad1e6")
	Dim      = lipgloss.Color("#5c6370")
	Fg       = lipgloss.Color("#c8ccd4")
	Bright   = lipgloss.Color("#e6e8ec")
	SelBg    = lipgloss.Color("#808080")
)

// Package hues, assigned in work-package order.
var Hues = []lipgloss.Color{
	lipgloss.Color("#5ad1e6"),
	lipgloss.Color("#c58af9"),
	lipgloss.Color("#8fd15a"),
	lipgloss.Color("#e86a9b"),
	lipgloss.Color("#f0a35e"),
}

var (
	S       = lipgloss.NewStyle()
	DimS    = S.Copy().Foreground(Dim)
	FgS     = S.Copy().Foreground(Fg)
	BrightS = S.Copy().Foreground(Bright)
	AccentS = S.Copy().Foreground(Accent)
	SelS    = S.Copy().Background(SelBg)
)

func StatusColor(s store.Status) lipgloss.Color {
	switch s {
	case store.Ready:
		return Ready
	case store.Blocked:
		return Blocked
	case store.Deferred:
		return Deferred
	case store.Done:
		return Done
	}
	return Dim
}

func Status(s store.Status) lipgloss.Style { return S.Copy().Foreground(StatusColor(s)) }

// StepGlyph is the one-cell marker for a step's status.
func StepGlyph(s store.Status) string {
	switch s {
	case store.Done:
		return "√"
	case store.Ready:
		return "○"
	case store.Deferred:
		return "~"
	}
	return "·"
}
