package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"flowcli/internal/styles"
)

// screenKeys defines the keybindings shown in the help/status line for each
// screen. It satisfies bubbles help.KeyMap.
type screenKeys struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Tab      key.Binding
	Enter    key.Binding
	Back     key.Binding
	Find     key.Binding
	Status   key.Binding
	Verify   key.Binding
	Expand   key.Binding
	Activity key.Binding
	Focus    key.Binding
	NextWP   key.Binding
	Write    key.Binding
	ScrollD  key.Binding
	Tree     key.Binding
	Lanes    key.Binding
	Chain    key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func kb(keys []string, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], desc))
}

// ShortHelp lists the first-class bindings for the one-line status bar.
func (s screenKeys) ShortHelp() []key.Binding {
	var out []key.Binding
	for _, kk := range []key.Binding{
		s.Up, s.Down, s.Left, s.Right, s.Tab, s.Enter, s.Back,
		s.Find, s.Status, s.Verify, s.Expand, s.Activity,
		s.Focus, s.NextWP, s.Write, s.ScrollD, s.Help,
	} {
		if kk.Enabled() {
			out = append(out, kk)
		}
	}
	return out
}

// FullHelp groups bindings into columns for the expanded help overlay.
func (s screenKeys) FullHelp() [][]key.Binding {
	var groups [][]key.Binding
	for _, g := range [][]key.Binding{
		{s.Up, s.Down, s.Left, s.Right, s.Tab, s.Enter, s.Back},
		{s.Find, s.Status, s.Verify, s.Expand, s.Activity, s.Focus, s.NextWP, s.Write, s.ScrollD},
		{s.Tree, s.Lanes, s.Chain, s.Help, s.Quit},
	} {
		var col []key.Binding
		for _, kk := range g {
			if kk.Enabled() {
				col = append(col, kk)
			}
		}
		if len(col) > 0 {
			groups = append(groups, col)
		}
	}
	return groups
}

func treeKeys() screenKeys {
	return screenKeys{
		Up:     kb([]string{"j", "up", "k", "down"}, "move"),
		Left:   kb([]string{"h", "left"}, "fold"),
		Right:  kb([]string{"l", "right"}, "expand"),
		Enter:  kb([]string{"enter"}, "detail"),
		Find:   kb([]string{"/"}, "find"),
		Status: kb([]string{"s"}, "status"),
		Lanes:  kb([]string{"2"}, "lanes"),
		Chain:  kb([]string{"3"}, "chain"),
		Tree:   kb([]string{"1"}, "tree"),
		Help:   kb([]string{"?"}, "toggle help"),
		Quit:   kb([]string{"q", "ctrl+c"}, "quit"),
	}
}

func lanesKeys() screenKeys {
	km := treeKeys()
	km.Up = kb([]string{"j", "down", "k", "up"}, "card")
	km.Left = kb([]string{"h", "left"}, "lane")
	km.Right = kb([]string{"l", "right", "tab"}, "lane")
	km.Lanes = kb([]string{"2"}, "lanes")
	return km
}

func chainKeys() screenKeys {
	km := treeKeys()
	km.Up = kb([]string{"j", "down", "k", "up"}, "move")
	km.Left = key.Binding{}
	km.Right = key.Binding{}
	km.Focus = kb([]string{"f"}, "focus")
	km.NextWP = kb([]string{"w"}, "next package")
	km.Chain = kb([]string{"3"}, "chain")
	return km
}

func detailKeys() screenKeys {
	km := treeKeys()
	km.Up = kb([]string{"j", "up", "k", "down"}, "move")
	km.Left = key.Binding{}
	km.Right = key.Binding{}
	km.Tab = kb([]string{"tab"}, "expand step")
	km.Verify = kb([]string{"v"}, "verify flag")
	km.Activity = kb([]string{"a"}, "activity")
	km.Back = kb([]string{"esc"}, "back")
	return km
}

func activityKeys() screenKeys {
	km := detailKeys()
	km.Up = kb([]string{"j", "up", "k", "down"}, "scroll")
	km.Write = kb([]string{"i"}, "write")
	km.ScrollD = kb([]string{"j", "up", "k", "down"}, "scroll")
	return km
}

// switcher renders the constant 1/2/3 view-switch hint (content only, no
// leading padding) with the currently active view dimmed and the others
// bright.
func switcher(active Screen) string {
	seg := func(label, name string, isActive bool) string {
		if isActive {
			return styles.DimS.Render(label + name)
		}
		return styles.BrightS.Render(label + name)
	}
	return seg("3 ", "chain", active == ScreenChain) + "  " +
		seg("2 ", "lanes", active == ScreenLanes) + "  " +
		seg("1 ", "tree", active == ScreenTree)
}

// statusLine builds the single status/help row for `active` screen: the short
// keybindings on the left (truncated to fit) and the view switcher flush to
// the right edge. The result is exactly `inner` cells wide so the frame's
// right wall closes.
func (m Model) statusLine(km screenKeys, active Screen, inner int) string {
	right := switcher(active)
	rightW := wlen(right)
	avail := inner - rightW - 1 // reserve 1 gap column
	if avail < 10 {
		avail = 10
	}
	m.help.Width = avail
	left := m.help.ShortHelpView(km.ShortHelp())
	fill := avail - wlen(left)
	if fill < 0 {
		fill = 0
	}
	return left + strings.Repeat(" ", fill) + " " + right
}
