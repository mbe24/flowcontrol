package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
	"flowcli/internal/styles"
)

// createKind is the node kind the create dialog will produce.
type createKind int

const (
	createProject createKind = iota
	createPackage
	createTask
	createStep
)

var createKindNames = []string{"project", "package", "task", "step"}

// createState is everything the create dialog needs. It lives as one field on
// Model so model.go gains a single line.
type createState struct {
	active bool
	kind   createKind
	// kindLocked hides the kind selector when the entry point already decided
	// (e.g. "add a step to this task").
	kindLocked bool
	parentID   string
	// parentTitle is the display title of the chosen parent (a WP for tasks,
	// a task for steps); empty means no parent chosen yet.
	parentTitle string
	// parentFocus marks the parent combobox row as the active element, ahead
	// of the form fields.
	parentFocus bool
	// parentQuery is the live filter text of the open picker.
	parentQuery string
	// parentItems are the candidate parents matching the query (a WP for
	// tasks, a task for steps), with parentIdx the highlighted one.
	parentItems []parentItem
	parentIdx   int

	form    Form
	another bool
	errAt   int
}

// createSpecs returns the field set for a kind. Fields that do not apply are
// absent rather than disabled — a greyed row in a 40-column box is noise.
func createSpecs(k createKind, title string) []fieldSpec {
	switch k {
	case createProject:
		return []fieldSpec{
			{label: "name", placeholder: "Fleet Dashboard", value: title},
			{label: "description", hint: "optional", placeholder: "What is this project for?", multiline: true},
		}
	case createStep:
		return []fieldSpec{
			{label: "title", placeholder: "What needs doing?", value: title},
			{label: "condition", hint: "optional · the agent verifies this", placeholder: "pnpm test:auth --grep rotate", mono: true},
		}
	case createTask:
		return []fieldSpec{
			{label: "title", placeholder: "What needs doing?", value: title},
			{label: "description", hint: "optional", placeholder: "What does done look like?", multiline: true},
			{label: "condition", hint: "optional · the agent verifies this", placeholder: "pnpm test:auth --grep rotate", mono: true},
		}
	default: // package
		return []fieldSpec{
			{label: "title", placeholder: "Fleet telemetry ingest", value: title},
			{label: "description", hint: "optional", placeholder: "What does done look like?", multiline: true},
		}
	}
}

const createWidth = 44

// parentItem is one candidate parent in the create dialog's searchable
// dropdown: a work package when creating a task, a task when creating a step.
// The filter matches the id and the title.
type parentItem struct {
	id, title string
}

func (p parentItem) Title() string       { return p.id + "  " + p.title }
func (p parentItem) Description() string { return "" }
func (p parentItem) FilterValue() string { return p.id + " " + p.title }

// parentLabel is the field label for the parent combobox: what the node under
// construction is attached to.
func (c createState) parentLabel() string {
	if c.kind == createStep {
		return "task"
	}
	return "work package"
}

// parentCandidates lists the nodes the current kind may attach to, within the
// current project: WPs for tasks, tasks for steps.
func (m Model) parentCandidates(k createKind) []store.Node {
	var out []store.Node
	for _, n := range m.nodes {
		if n.ProjectID != m.projectID {
			continue
		}
		switch k {
		case createTask:
			if n.Type == store.WorkPackage {
				out = append(out, n)
			}
		case createStep:
			if n.Type == store.Task {
				out = append(out, n)
			}
		}
	}
	return out
}

// rebuildParentItems filters the candidate parents (WPs for tasks, tasks for
// steps) by the current query and re-selects the contextual parent when it is
// still among them. The highlighted row tracks parentIdx.
func (m *Model) rebuildParentItems() {
	c := &m.create
	c.parentItems = c.parentItems[:0]
	q := strings.ToLower(c.parentQuery)
	for _, n := range m.parentCandidates(c.kind) {
		it := parentItem{id: n.ID, title: n.Title}
		if q == "" || strings.Contains(strings.ToLower(it.id+" "+it.title), q) {
			c.parentItems = append(c.parentItems, it)
		}
	}
	// While filtering, land on the first match; otherwise honour the
	// contextual parent selection.
	if q != "" {
		c.parentIdx = 0
	} else {
		c.parentIdx = 0
		for i, it := range c.parentItems {
			if it.id == c.parentID {
				c.parentIdx = i
				break
			}
		}
	}
	if len(c.parentItems) > 0 && c.parentTitle == "" {
		c.parentTitle = c.parentItems[c.parentIdx].title
	}
}

// openParentDrop opens the searchable picker as its own dialog on top of the
// create form.
func (m *Model) openParentDrop() {
	c := &m.create
	c.parentQuery = ""
	m.rebuildParentItems()
	m.overlay = OverlayPickParent
}

// openCreate starts the dialog. parentID may be empty for project/package.
func (m *Model) openCreate(k createKind, parentID, title string, locked bool) {
	m.create = createState{
		active:     true,
		kind:       k,
		kindLocked: locked,
		parentID:   parentID,
		form:       newForm(createWidth, createSpecs(k, title)),
		another:    title != "", // arrived from an inline row: likely a run
		errAt:      -1,
	}
	// Tasks attach to a work package, steps to a task: start on the searchable
	// parent dropdown so the attachment is explicit.
	if k == createTask || k == createStep {
		m.rebuildParentItems()
		m.create.parentFocus = true
	}
	m.overlay = OverlayCreate
}

func (m *Model) closeCreate() {
	m.create = createState{errAt: -1}
	m.overlay = OverlayNone
}

// switchKind rebuilds the form, carrying the title across.
func (m *Model) switchKind(k createKind) {
	title := m.create.form.Value(0)
	m.create.kind = k
	m.create.form = newForm(createWidth, createSpecs(k, title))
	m.create.errAt = -1
	// Different kind → different parent pool. Drop any stale selection and
	// rebuild the combobox (or leave the field out when irrelevant).
	m.create.parentID = ""
	m.create.parentTitle = ""
	m.create.parentQuery = ""
	if k == createTask || k == createStep {
		m.rebuildParentItems()
		m.create.parentFocus = true
	} else {
		m.create.parentFocus = false
	}
}

func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.create

	switch msg.String() {
	case "esc":
		m.closeCreate()
		return m, nil

	case "tab":
		if c.parentFocus {
			c.parentFocus = false
			c.form.setFocus(0)
			return m, nil
		}
		c.form.Next()
		return m, nil
	case "shift+tab":
		if c.form.focus == 0 && (c.kind == createTask || c.kind == createStep) {
			c.parentFocus = true
			return m, nil
		}
		c.form.Prev()
		return m, nil

	case "ctrl+n":
		c.another = !c.another
		return m, nil

	// Kind selector, only while unlocked and only from the first field.
	case "left", "right":
		if !c.kindLocked && c.form.focus == 0 {
			d := 1
			if msg.String() == "left" {
				d = -1
			}
			k := int(c.kind) + d
			if k >= 1 && k <= 3 { // project is reached from the picker only
				m.switchKind(createKind(k))
				return m, nil
			}
		}

	case "enter", "down":
		if c.parentFocus {
			m.openParentDrop()
			return m, nil
		}
		// A textarea owns enter for newlines; commit with ctrl+s or from any
		// single-line field.
		if c.form.focus == c.form.areaAt {
			break
		}
		return m.submitCreate()

	case "ctrl+s":
		return m.submitCreate()
	}

	if c.parentFocus {
		// The parent row is not a text field: keys unrelated to the combobox
		// must not leak into the title input behind it.
		return m, nil
	}
	cmd := c.form.Update(msg)
	return m, cmd
}

// updateParentDrop handles keys while the parent dropdown is open: typing (or
// backspace) edits the live filter, up/down navigate the filtered candidates,
// enter picks the highlighted one, esc closes the dropdown without choosing.
func (m Model) updateParentDrop(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.create
	switch msg.String() {
	case "esc":
		c.parentQuery = ""
		c.parentFocus = true
		m.overlay = OverlayCreate
		return m, nil

	case "enter":
		if len(c.parentItems) > 0 {
			it := c.parentItems[c.parentIdx]
			c.parentID = it.id
			c.parentTitle = it.title
		}
		c.parentQuery = ""
		c.parentFocus = true
		m.overlay = OverlayCreate
		return m, nil

	case "up", "down":
		if len(c.parentItems) == 0 {
			return m, nil
		}
		if msg.String() == "down" {
			c.parentIdx = min(c.parentIdx+1, len(c.parentItems)-1)
		} else {
			c.parentIdx = max(c.parentIdx-1, 0)
		}
		return m, nil

	case "backspace":
		if c.parentQuery != "" {
			c.parentQuery = c.parentQuery[:len(c.parentQuery)-1]
		}
		m.rebuildParentItems()
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			c.parentQuery += string(msg.Runes)
			m.rebuildParentItems()
		}
		return m, nil
	}
}

func (m Model) submitCreate() (tea.Model, tea.Cmd) {
	c := &m.create
	title := c.form.Value(0)
	if title == "" {
		c.errAt = 0
		c.form.err = "a title is required"
		return m, nil
	}
	c.errAt = -1
	c.form.err = ""

	kind := c.kind
	parent := c.parentID
	another := c.another

	// Tasks attach to a work package, steps to a task: the parent must be
	// chosen from the searchable picker before the node can be created.
	if (kind == createTask || kind == createStep) && parent == "" {
		c.parentFocus = true
		c.parentQuery = ""
		m.rebuildParentItems()
		return m, nil
	}

	// Field order differs per kind; read by index against createSpecs.
	var desc, cond string
	switch kind {
	case createProject:
		desc = c.form.Value(1)
	case createStep:
		cond = c.form.Value(1)
	case createTask:
		desc = c.form.Value(2 - 1)
		cond = c.form.Value(2)
	default:
		desc = c.form.Value(1)
	}

	if another {
		c.form.Reset()
	} else {
		m.closeCreate()
	}

	return m, func() tea.Msg {
		if kind == createProject {
			if _, err := m.store.CreateProject(m.ctx, title, desc, true); err != nil {
				return refreshedMsg{err: err}
			}
			return m.refresh()
		}
		var t store.NodeType
		switch kind {
		case createPackage:
			t = store.WorkPackage
		case createTask:
			t = store.Task
		default:
			t = store.Step
		}
		in := store.NewNode{
			ProjectID: m.projectID,
			ParentID:  parent,
			Type:      t,
			Title:     title,
			Condition: cond,
		}
		if desc != "" {
			in.Description = []string{desc}
		}
		if _, err := m.store.CreateNode(m.ctx, in); err != nil {
			return refreshedMsg{err: err}
		}
		return m.refresh()
	}
}

func (m Model) viewCreate(w int) string {
	c := m.create
	var lines []string
	lines = append(lines, "")

	if !c.kindLocked {
		var seg strings.Builder
		seg.WriteString(styles.DimS.Render("kind   "))
		for i := 1; i <= 3; i++ {
			name := createKindNames[i]
			if createKind(i) == c.kind {
				seg.WriteString(styles.SelS.Render(" " + styles.AccentS.Render(name) + " "))
			} else {
				seg.WriteString(" " + styles.DimS.Render(name) + " ")
			}
			seg.WriteString(" ")
		}
		lines = append(lines, seg.String())
		lines = append(lines, "")
	}

	lines = append(lines, c.form.Rows(c.errAt)...)

	if c.kind == createTask || c.kind == createStep {
		// Searchable parent combobox: tasks attach to a work package, steps
		// to a task. The row is focusable and opens the dropdown on enter.
		label := styles.DimS.Render(c.parentLabel() + "  ")
		value := c.parentTitle
		if value == "" {
			value = "choose…"
		}
		if c.parentFocus {
			value = styles.AccentS.Render(value)
		} else {
			value = styles.FgS.Render(value)
		}
		lines = append(lines, "", label+value)
	} else if c.parentID != "" {
		if p, ok := m.byID[c.parentID]; ok {
			lines = append(lines, "", styles.DimS.Render("parent   ")+styles.FgS.Render(p.Title))
		}
	} else if c.kind == createPackage {
		lines = append(lines, "", styles.DimS.Render("project  ")+styles.FgS.Render(m.projectName()))
	}

	another := styles.DimS.Render("another")
	if c.another {
		another = styles.AccentS.Render("another")
	}
	keys := styles.AccentS.Render("⇥") + styles.DimS.Render(" field  ") +
		styles.AccentS.Render("^n") + " " + another + styles.DimS.Render("  ") +
		styles.AccentS.Render("↵") + styles.DimS.Render(" create  ") +
		styles.AccentS.Render("esc")

	title := "new " + createKindNames[c.kind]
	return boxWithKeys(title, styles.Accent, lines, keys, createWidth, w)
}

// pickWidth is the fixed content width of the parent picker dialog, a little
// wider than the create form so its list has room to breathe.
const pickWidth = 52

// pickRows is how many candidate rows the parent picker shows at once.
const pickRows = 4

// viewPickParent renders the searchable parent picker as its own dialog: a
// filter line on top, the filtered candidate list below. It is drawn over the
// create form (see View) so neither dialog grows with the candidate count.
func (m Model) viewPickParent(w int) string {
	c := m.create
	var lines []string
	lines = append(lines, "", "❯ "+styles.FgS.Render(pad(c.parentQuery, pickWidth-6)))
	lines = append(lines, styles.DimS.Render(strings.Repeat("─", pickWidth-4)))

	// pickRows candidate rows at a time; the highlighted one is selected. The
	// window scrolls so the dialog keeps a constant height.
	var rows []string
	if len(c.parentItems) == 0 {
		rows = append(rows, styles.DimS.Render("  no matches"))
	} else {
		start := c.parentIdx
		if start > len(c.parentItems)-pickRows {
			start = len(c.parentItems) - pickRows
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < start+pickRows && i < len(c.parentItems); i++ {
			it := c.parentItems[i]
			label := padTrunc(padTrunc(it.id, 9)+"  "+it.title, pickWidth-8)
			if i == c.parentIdx {
				rows = append(rows, styles.SelS.Render("▸ "+label))
			} else {
				rows = append(rows, "  "+styles.FgS.Render(label))
			}
		}
	}
	for len(rows) < pickRows {
		rows = append(rows, "")
	}
	lines = append(lines, rows...)

	// Paginator dots: one per visible page (pickRows results per page), the
	// current page filled so the user sees how many hits there are.
	if len(c.parentItems) > pickRows {
		pages := (len(c.parentItems) + pickRows - 1) / pickRows
		page := c.parentIdx / pickRows
		var dots strings.Builder
		for p := 0; p < pages; p++ {
			if p == page {
				dots.WriteString(styles.AccentS.Render("● "))
			} else {
				dots.WriteString(styles.DimS.Render("· "))
			}
		}
		lines = append(lines, "  "+dots.String())
	}

	keys := styles.DimS.Render("type to filter  ") +
		styles.AccentS.Render("↑↓") + styles.DimS.Render(" move  ") +
		styles.AccentS.Render("↵") + styles.DimS.Render(" pick  ") +
		styles.AccentS.Render("esc") + styles.DimS.Render(" back")
	return boxWithKeys("select "+c.parentLabel(), styles.Accent, lines, keys, pickWidth, w)
}
