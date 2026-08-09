package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

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
	Projects key.Binding
	ToggleDone key.Binding
	Undo     key.Binding
	Create   key.Binding
	Child    key.Binding
	Sibling  key.Binding
	Edit     key.Binding
	EditStep key.Binding
	DepAdd   key.Binding
}

func kb(helpKey string, keys []string, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, desc))
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
		Up:     kb("j / k", []string{"j", "up", "k", "down"}, "move"),
		Left:   kb("h", []string{"h", "left"}, "fold"),
		Right:  kb("l", []string{"l", "right"}, "expand"),
		Enter:  kb("enter", []string{"enter"}, "open detail"),
		Find:   kb("/", []string{"/"}, "find"),
		Status: kb("s", []string{"s"}, "status"),
		Lanes:  kb("2", []string{"2"}, "lanes"),
		Chain:  kb("3", []string{"3"}, "chain"),
		Tree:   kb("1", []string{"1"}, "tree"),
		Help:   kb("?", []string{"?"}, "toggle help"),
		Quit:   kb("q", []string{"q", "ctrl+c"}, "quit"),
		Back:   kb("esc", []string{"esc"}, "back up"),
		Projects: kb("p", []string{"p"}, "projects panel"),
		ToggleDone: kb("D", []string{"D"}, "toggle done"),
		Undo:   kb("u", []string{"u"}, "undo last change"),
		Create: kb("c", []string{"c"}, "create node"),
		Child:  kb("O", []string{"O"}, "new child"),
		Sibling: kb("o", []string{"o"}, "new sibling"),
		Edit:   key.Binding{}, // c -> edit is only set in detailKeys
	}
}

func lanesKeys() screenKeys {
	km := treeKeys()
	km.Up = kb("j / k", []string{"j", "down", "k", "up"}, "move card")
	km.Left = kb("h", []string{"h", "left"}, "prev lane")
	km.Right = kb("l / tab", []string{"l", "right", "tab"}, "next lane")
	km.Lanes = kb("2", []string{"2"}, "lanes")
	return km
}

func chainKeys() screenKeys {
	km := treeKeys()
	km.Up = kb("j / k", []string{"j", "down", "k", "up"}, "move up")
	km.Create = key.Binding{} // no node creation from the chain view
	km.Left = key.Binding{}
	km.Right = key.Binding{}
	km.Focus = kb("f", []string{"f"}, "focus task")
	km.NextWP = kb("w", []string{"w"}, "next package")
	km.Chain = kb("3", []string{"3"}, "chain")
	return km
}

func detailKeys() screenKeys {
	km := treeKeys()
	km.Up = kb("j / k", []string{"j", "up", "k", "down"}, "move cursor")
	km.Left = key.Binding{}
	km.Right = key.Binding{}
	km.Tab = kb("tab", []string{"tab"}, "focus deps")
	km.Enter = kb("enter", []string{"enter"}, "expand step")
	km.Verify = kb("v", []string{"v"}, "verify step")
	km.Activity = kb("a", []string{"a"}, "activity panel")
	km.Back = kb("esc", []string{"esc"}, "back up")
	km.Create = key.Binding{} // c in detail means edit, not create
	km.Child = kb("O", []string{"O"}, "new child")
	km.Sibling = kb("o", []string{"o"}, "new sibling")
	km.Edit = kb("c", []string{"c"}, "edit title & condition")
	km.EditStep = kb("C", []string{"C"}, "edit step")
	km.DepAdd = kb("y", []string{"y"}, "add dependency")
	return km
}

func activityKeys() screenKeys {
	km := detailKeys()
	// activity has no create/edit bindings — silence the inherited ones
	km.Create = key.Binding{}
	km.Child = key.Binding{}
	km.Edit = key.Binding{}
	km.EditStep = key.Binding{}
	km.DepAdd = key.Binding{}
	km.Up = kb("j / k", []string{"j", "up", "k", "down"}, "scroll up")
	km.Write = kb("i", []string{"i"}, "write message")
	km.ScrollD = kb("j / k", []string{"j", "up", "k", "down"}, "scroll down")
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
	return seg("1 ", "tree", active == ScreenTree) + "  " +
		seg("2 ", "lanes", active == ScreenLanes) + "  " +
		seg("3 ", "chain", active == ScreenChain)
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
	// Don't rely solely on help's internal truncation — bubbles v1.0.0 can
	// let an oversized final item blow past the width. Cap it to `avail`
	// ourselves (ANSI-preserving) so the row always closes at `inner`.
	if wlen(left) > avail {
		left = truncANSI(left, avail)
	}
	fill := avail - wlen(left)
	if fill < 0 {
		fill = 0
	}
	return left + strings.Repeat(" ", fill) + " " + right
}

// kmForScreen returns the key map for the given screen, used both for the
// status-line short help and the full "?" help dialog.
func kmForScreen(s Screen) screenKeys {
	switch s {
	case ScreenLanes:
		return lanesKeys()
	case ScreenChain:
		return chainKeys()
	case ScreenDetail:
		return detailKeys()
	case ScreenActivity:
		return activityKeys()
	default:
		return treeKeys()
	}
}

// viewHelpPanel renders the "?" help as a borderless, full-screen key map. It
// lays out three equally-wide lanes (MOVE, ACT, FIND & SCOPE) side by side
// across the full terminal width, each with a coloured title, a grey divider,
// and colour-tinted keys with grey descriptions on an aligned key column. It is
// drawn flush to the top of the terminal with no frame around it.
func (m Model) viewHelpPanel(w, h int) string {
	km := kmForScreen(m.screen)

	cols := []lipgloss.Color{styles.Hues[0], styles.Hues[2], styles.Hues[4]}

	lanes := []struct {
		title string
		col   lipgloss.Color
		binds []key.Binding
	}{
		{"MOVE", cols[0], []key.Binding{km.Up, km.Down, km.Left, km.Right, km.Tab, km.Enter, km.Back}},
		{"ACT", cols[1], []key.Binding{km.Status, km.Verify, km.Expand, km.Activity, km.Focus, km.NextWP, km.Write, km.ScrollD, km.Undo, km.Create, km.Child, km.Sibling, km.Edit, km.EditStep, km.DepAdd}},
		{"FIND & SCOPE", cols[2], []key.Binding{km.Find, km.Projects, km.Tree, km.ToggleDone, km.Lanes, km.Chain, km.Help, km.Quit}},
	}

	type laneData struct {
		keys, descs  []string // plain
		stKeys, stDs []string // styled
		keyW         int
	}
	data := make([]laneData, 3)
	maxRows := 0
	for i, l := range lanes {
		var ks, ds, sK, sD []string
		for _, kk := range l.binds {
			if !kk.Enabled() {
				continue
			}
			h := kk.Help()
			ks = append(ks, h.Key)
			ds = append(ds, h.Desc)
			sK = append(sK, styles.S.Copy().Foreground(l.col).Render(h.Key))
			sD = append(sD, styles.DimS.Render(h.Desc))
		}
		keyW := 0
		for _, k := range ks {
			if v := wlen(k); v > keyW {
				keyW = v
			}
		}
		data[i] = laneData{keys: ks, descs: ds, stKeys: sK, stDs: sD, keyW: keyW}
		if len(ks) > maxRows {
			maxRows = len(ks)
		}
	}

	// Divide the full width among the three lanes, with gutters between.
	const gutter = 6
	fullW := w
	innerAvail := fullW - 2*gutter
	laneW := innerAvail / 3
	if laneW < 18 {
		laneW = 18
	}
	// Description starts keyGap cells after the (padded) key column.
	const keyGap = 2

	var body []string

	// Title row: coloured titles, flush-left within each lane.
	var titleLine strings.Builder
	for i := range lanes {
		if i > 0 {
			titleLine.WriteString(strings.Repeat(" ", gutter))
		}
		title := lanes[i].title
		tw := wlen(title)
		tail := laneW - tw
		if tail < 0 {
			tail = 0
		}
		titleLine.WriteString(styles.S.Copy().Foreground(cols[i]).Render(title) + strings.Repeat(" ", tail))
	}
	body = append(body, titleLine.String())

	// Grey rule row spanning each lane's full width, flush-left.
	var rl strings.Builder
	for i := range lanes {
		if i > 0 {
			rl.WriteString(strings.Repeat(" ", gutter))
		}
		rl.WriteString(styles.DimS.Render(strings.Repeat("─", laneW)))
	}
	body = append(body, rl.String())

	// Binding rows. Every lane cell is exactly laneW cells wide so the columns
	// line up; keys are padded to the lane's keyW and the description follows.
	for r := 0; r < maxRows; r++ {
		var line strings.Builder
		for i := range data {
			if i > 0 {
				line.WriteString(strings.Repeat(" ", gutter))
			}
			if r < len(data[i].keys) {
				keyPad := data[i].keyW - wlen(data[i].keys[r])
				if keyPad < 0 {
					keyPad = 0
				}
				descW := wlen(data[i].descs[r])
				tailPad := laneW - data[i].keyW - keyGap - descW
				if tailPad < 0 {
					tailPad = 0
				}
				line.WriteString(data[i].stKeys[r])
				line.WriteString(strings.Repeat(" ", keyPad+keyGap))
				line.WriteString(data[i].stDs[r])
				line.WriteString(strings.Repeat(" ", tailPad))
			} else {
				line.WriteString(strings.Repeat(" ", laneW))
			}
		}
		body = append(body, line.String())
	}

	// Draw flush to the top (no frame, no vertical centering).
	return strings.Join(body, "\n") + "\n"
}
