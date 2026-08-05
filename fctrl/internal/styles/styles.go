package styles

import (
	"github.com/charmbracelet/lipgloss"

	"flowcontrol/fctrl/internal/store"
)

// The four status hues plus focus, matching the web app. Hex values degrade to
// the nearest ANSI colour automatically on limited terminals.
var (
	Ready    = lipgloss.Color("#3ecf8e")
	Blocked  = lipgloss.Color("#ef6a5a")
	Deferred = lipgloss.Color("#7c8394")
	Done     = lipgloss.Color("#5f87af")
	Focus    = lipgloss.Color("#5ad1e6")
	FG       = lipgloss.Color("#e6e8ec")
	Muted    = lipgloss.Color("#8a909b")
	Dim      = lipgloss.Color("#565c66")
	Rule     = lipgloss.Color("#2b2f36")
	CursorBG = lipgloss.Color("#1b2a30")
)

var (
	Title     = lipgloss.NewStyle().Foreground(Focus)
	Head      = lipgloss.NewStyle().Foreground(FG).Bold(true)
	Body      = lipgloss.NewStyle().Foreground(FG)
	Soft      = lipgloss.NewStyle().Foreground(Muted)
	Faint     = lipgloss.NewStyle().Foreground(Dim)
	RuleStyle = lipgloss.NewStyle().Foreground(Rule)
	Cursor    = lipgloss.NewStyle().Background(CursorBG).Foreground(FG)
	Box       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Rule).Padding(0, 1)
	FocusBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Focus).Padding(0, 1)
	OKBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Ready).Padding(0, 1)
	Label     = lipgloss.NewStyle().Foreground(Dim)
)

func StatusColor(s store.Status) lipgloss.Color {
	switch s {
	case store.StatusReady:
		return Ready
	case store.StatusBlocked:
		return Blocked
	case store.StatusDeferred:
		return Deferred
	case store.StatusDone:
		return Done
	}
	return Muted
}

func StatusStyle(s store.Status) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(StatusColor(s))
}

// Glyph is the colour-blind fallback: shape carries the status too.
func Glyph(s store.Status) string {
	switch s {
	case store.StatusReady:
		return "●"
	case store.StatusBlocked:
		return "●"
	case store.StatusDeferred:
		return "⏸"
	case store.StatusDone:
		return "●"
	}
	return "·"
}

func StepGlyph(s store.Status) string {
	switch s {
	case store.StatusDone:
		return "✓"
	case store.StatusReady:
		return "○"
	case store.StatusDeferred:
		return "⏸"
	}
	return "·"
}

func VerifyGlyph(r store.VerifyResult) (string, lipgloss.Color) {
	switch r {
	case store.VerifyPass:
		return "✓", Ready
	case store.VerifyFail:
		return "✕", Blocked
	case store.VerifyStale:
		return "◷", Muted
	}
	return "–", Dim
}

// Bar renders a 10-cell progress bar in the done/ready/blocked hues.
func Bar(done, ready, blocked, total, cells int) string {
	if total <= 0 || cells <= 0 {
		return lipgloss.NewStyle().Foreground(Rule).Render(repeat("░", cells))
	}
	d := done * cells / total
	r := ready * cells / total
	b := blocked * cells / total
	rest := cells - d - r - b
	if rest < 0 {
		rest = 0
	}
	out := lipgloss.NewStyle().Foreground(Done).Render(repeat("▇", d))
	out += lipgloss.NewStyle().Foreground(Ready).Render(repeat("▇", r))
	out += lipgloss.NewStyle().Foreground(Blocked).Render(repeat("▇", b))
	out += lipgloss.NewStyle().Foreground(Rule).Render(repeat("░", rest))
	return out
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
