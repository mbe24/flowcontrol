package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/styles"
)

// Form is the canonical bubbles multi-input pattern: a slice of inputs plus a
// focus index, with tab/shift-tab cycling. Only the focused field receives
// Update; the rest merely render.
//
// One textarea is allowed, at the index named by areaAt (-1 for none), because
// description is the only multi-line field in the app.
type Form struct {
	labels []string
	hints  []string
	inputs []textinput.Model
	area   textarea.Model
	areaAt int

	focus int
	err   string
	// width is the content width inside the dialog border.
	width int
}

type fieldSpec struct {
	label       string
	hint        string
	placeholder string
	value       string
	multiline   bool
	mono        bool
}

func newForm(width int, specs []fieldSpec) Form {
	f := Form{width: width, areaAt: -1}
	for i, sp := range specs {
		f.labels = append(f.labels, sp.label)
		f.hints = append(f.hints, sp.hint)
		if sp.multiline {
			ta := textarea.New()
			ta.Placeholder = sp.placeholder
			ta.SetValue(sp.value)
			ta.SetWidth(width - 2)
			ta.SetHeight(3)
			ta.ShowLineNumbers = false
			ta.CharLimit = 2000
			f.area = ta
			f.areaAt = i
			f.inputs = append(f.inputs, textinput.Model{}) // placeholder slot
			continue
		}
		ti := textinput.New()
		ti.Placeholder = sp.placeholder
		ti.SetValue(sp.value)
		// Width and CharLimit must be set: left unbounded both grow past the
		// border and the frame stops closing.
		ti.Width = width - 4
		ti.CharLimit = 200
		ti.Prompt = ""
		f.inputs = append(f.inputs, ti)
	}
	f.setFocus(0)
	return f
}

func (f *Form) setFocus(i int) {
	n := len(f.labels)
	if n == 0 {
		return
	}
	f.focus = ((i % n) + n) % n
	for j := range f.inputs {
		if j == f.areaAt {
			continue
		}
		if j == f.focus {
			f.inputs[j].Focus()
		} else {
			f.inputs[j].Blur()
		}
	}
	if f.areaAt >= 0 {
		if f.focus == f.areaAt {
			f.area.Focus()
		} else {
			f.area.Blur()
		}
	}
}

func (f *Form) Next()  { f.setFocus(f.focus + 1) }
func (f *Form) Prev()  { f.setFocus(f.focus - 1) }
func (f *Form) Clear() { f.err = "" }

// Value returns the trimmed content of field i.
func (f Form) Value(i int) string {
	if i == f.areaAt {
		return strings.TrimSpace(f.area.Value())
	}
	if i < 0 || i >= len(f.inputs) {
		return ""
	}
	return strings.TrimSpace(f.inputs[i].Value())
}

func (f *Form) Reset() {
	for i := range f.inputs {
		if i == f.areaAt {
			continue
		}
		f.inputs[i].SetValue("")
	}
	if f.areaAt >= 0 {
		f.area.SetValue("")
	}
	f.err = ""
	f.setFocus(0)
}

// Update routes the key to the focused field only.
func (f *Form) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if f.focus == f.areaAt {
		f.area, cmd = f.area.Update(msg)
		return cmd
	}
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd
}

// Rows renders the form as dialog body lines: label, bordered field, and the
// error line under whichever field failed validation.
func (f Form) Rows(errAt int) []string {
	var out []string
	for i, label := range f.labels {
		head := styles.DimS.Render(label)
		if f.hints[i] != "" {
			head += "  " + styles.DimS.Render(f.hints[i])
		}
		out = append(out, head)

		border := styles.Dim
		if i == f.focus {
			border = styles.Accent
		}
		if i == errAt && f.err != "" {
			border = styles.Blocked
		}
		bs := styles.S.Copy().Foreground(border)

		inner := f.width - 4
		out = append(out, bs.Render("╭"+strings.Repeat("─", inner+2)+"╮"))
		for _, line := range f.fieldLines(i, inner) {
			out = append(out, bs.Render("│")+" "+line+" "+bs.Render("│"))
		}
		out = append(out, bs.Render("╰"+strings.Repeat("─", inner+2)+"╯"))

		if i == errAt && f.err != "" {
			out = append(out, styles.S.Copy().Foreground(styles.Blocked).Render(f.err))
		}
		out = append(out, "")
	}
	if len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out
}

func (f Form) fieldLines(i, inner int) []string {
	if i == f.areaAt {
		raw := strings.Split(f.area.View(), "\n")
		var out []string
		for _, r := range raw {
			out = append(out, padTrunc(r, inner))
		}
		return out
	}
	return []string{padTrunc(f.inputs[i].View(), inner)}
}
