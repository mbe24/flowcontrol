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
	form       Form
	another    bool
	errAt      int
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
}

func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.create
	switch msg.String() {
	case "esc":
		m.closeCreate()
		return m, nil

	case "tab":
		c.form.Next()
		return m, nil
	case "shift+tab":
		c.form.Prev()
		return m, nil

	case "ctrl+n":
		c.another = !c.another
		return m, nil

	// Kind selector, only while unlocked and only from the first field.
	case "left", "right":
		if !c.kindLocked {
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

	case "enter":
		// A textarea owns enter for newlines; commit with ctrl+s or from any
		// single-line field.
		if c.form.focus == c.form.areaAt {
			break
		}
		return m.submitCreate()

	case "ctrl+s":
		return m.submitCreate()
	}

	cmd := c.form.Update(msg)
	return m, cmd
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

	if c.parentID != "" {
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
