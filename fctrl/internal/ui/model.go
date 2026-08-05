package ui

import (
	"context"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"flowcontrol/fctrl/internal/store"
)

type screen int

const (
	screenProjects screen = iota
	screenBrowser
	screenHelp
)

type mode int

const (
	modeNone mode = iota
	modeFinder
	modeStatus
	modeConfirm
)

type rowKind int

const (
	rowWP rowKind = iota
	rowTask
	rowStep
	rowArchived
)

type row struct {
	kind    rowKind
	node    store.Node
	depth   int
	hasKids bool
	label   string
}

type change struct {
	nodeID string
	prev   store.Status
}

type result struct {
	kind  string // "task" | "step" | "cmd"
	id    string
	title string
	hint  string
	node  store.Node
}

type projectsMsg struct{ ps []store.Project }
type dataMsg struct {
	nodes []store.Node
	deps  []store.Dependency
}
type verifiedMsg struct {
	id  string
	res store.VerifyResult
}
type errMsg struct{ err error }

// Model is the whole TUI. One model, five screens, two overlays.
type Model struct {
	st  store.Store
	ctx context.Context

	w, h int

	screen screen
	mode   mode

	projects   []store.Project
	projCursor int
	projectID  string

	nodes    []store.Node
	deps     []store.Dependency
	byID     map[string]store.Node
	blockers map[string][]string
	blocks   map[string][]string

	rows         []row
	cursor       int
	expanded     map[string]bool
	showArchived bool
	focusDetail  bool

	finder  textinput.Model
	results []result
	fCursor int

	statusCursor int
	pending      store.Status

	spin      spinner.Model
	verifying bool
	flash     string

	last *change
	err  error
}

func New(st store.Store) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "task, step or command"
	ti.CharLimit = 64

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		st:       st,
		ctx:      context.Background(),
		screen:   screenProjects,
		expanded: map[string]bool{},
		byID:     map[string]store.Node{},
		blockers: map[string][]string{},
		blocks:   map[string][]string{},
		finder:   ti,
		spin:     sp,
		w:        120,
		h:        36,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadProjects(m.st), m.spin.Tick)
}

func loadProjects(st store.Store) tea.Cmd {
	return func() tea.Msg {
		ps, err := st.Projects(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return projectsMsg{ps}
	}
}

func loadData(st store.Store, pid string) tea.Cmd {
	return func() tea.Msg {
		ns, err := st.Nodes(context.Background(), pid)
		if err != nil {
			return errMsg{err}
		}
		ds, err := st.Dependencies(context.Background(), pid)
		if err != nil {
			return errMsg{err}
		}
		return dataMsg{ns, ds}
	}
}

func applyStatus(st store.Store, pid, nodeID string, s store.Status) tea.Cmd {
	return func() tea.Msg {
		if err := st.SetStatus(context.Background(), nodeID, s); err != nil {
			return errMsg{err}
		}
		ns, err := st.Nodes(context.Background(), pid)
		if err != nil {
			return errMsg{err}
		}
		ds, err := st.Dependencies(context.Background(), pid)
		if err != nil {
			return errMsg{err}
		}
		return dataMsg{ns, ds}
	}
}

func verifyNode(st store.Store, id string) tea.Cmd {
	return func() tea.Msg {
		res, err := st.Verify(context.Background(), id)
		if err != nil {
			return errMsg{err}
		}
		return verifiedMsg{id: id, res: res}
	}
}

func (m *Model) index() {
	m.byID = map[string]store.Node{}
	for _, n := range m.nodes {
		m.byID[n.ID] = n
	}
	m.blockers = map[string][]string{}
	m.blocks = map[string][]string{}
	for _, d := range m.deps {
		m.blockers[d.BlockedID] = append(m.blockers[d.BlockedID], d.BlockerID)
		m.blocks[d.BlockerID] = append(m.blocks[d.BlockerID], d.BlockedID)
	}
	m.buildRows()
}

func (m *Model) buildRows() {
	wps := []store.Node{}
	for _, n := range m.nodes {
		if n.Type == store.TypeWorkPackage {
			wps = append(wps, n)
		}
	}
	sort.SliceStable(wps, func(i, j int) bool { return statePriority(wps[i].State) < statePriority(wps[j].State) })

	children := func(parent string, t store.NodeType) []store.Node {
		out := []store.Node{}
		for _, n := range m.nodes {
			if n.ParentID == parent && n.Type == t {
				out = append(out, n)
			}
		}
		return out
	}

	rows := []row{}
	hidden := 0
	for _, wp := range wps {
		if !m.showArchived && (wp.State == store.StateDone || wp.State == store.StateArchived) {
			hidden++
			continue
		}
		tasks := children(wp.ID, store.TypeTask)
		rows = append(rows, row{kind: rowWP, node: wp, hasKids: len(tasks) > 0})
		if !m.expanded[wp.ID] {
			continue
		}
		for _, t := range tasks {
			steps := children(t.ID, store.TypeStep)
			rows = append(rows, row{kind: rowTask, node: t, depth: 1, hasKids: len(steps) > 0})
			if !m.expanded[t.ID] {
				continue
			}
			for _, s := range steps {
				rows = append(rows, row{kind: rowStep, node: s, depth: 2})
			}
		}
	}
	if hidden > 0 {
		rows = append(rows, row{kind: rowArchived, label: plural(hidden, "done package", "done packages") + " hidden"})
	}
	m.rows = rows
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func statePriority(s store.WPState) int {
	switch s {
	case store.StateActive:
		return 0
	case store.StatePlanned:
		return 1
	case store.StateDone:
		return 2
	}
	return 3
}

func (m Model) current() (store.Node, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return store.Node{}, false
	}
	r := m.rows[m.cursor]
	if r.kind == rowArchived {
		return store.Node{}, false
	}
	return r.node, true
}

// detailNode is what the right-hand pane shows: the task under the cursor, or
// the task owning the step under the cursor.
func (m Model) detailNode() (store.Node, bool) {
	n, ok := m.current()
	if !ok {
		return n, false
	}
	if n.Type == store.TypeStep {
		if parent, ok := m.byID[n.ParentID]; ok {
			return parent, true
		}
	}
	return n, true
}

func (m Model) projectName() string {
	for _, p := range m.projects {
		if p.ID == m.projectID {
			return p.Name
		}
	}
	return "—"
}

func (m *Model) expandActive() {
	for _, n := range m.nodes {
		if n.Type == store.TypeWorkPackage && n.State == store.StateActive {
			m.expanded[n.ID] = true
		}
	}
}

func (m *Model) search(q string) {
	m.results = nil
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return
	}
	for _, n := range m.nodes {
		if n.Type == store.TypeWorkPackage {
			continue
		}
		if !strings.Contains(strings.ToLower(n.Title), q) {
			continue
		}
		kind := "task"
		hint := ""
		if n.Type == store.TypeStep {
			kind = "step"
			if p, ok := m.byID[n.ParentID]; ok {
				hint = p.ID
			}
		} else if wp, ok := m.byID[n.ParentID]; ok {
			hint = shorten(wp.Title, 16)
		}
		m.results = append(m.results, result{kind: kind, id: n.ID, title: n.Title, hint: hint, node: n})
		if len(m.results) >= 8 {
			break
		}
	}
	for _, s := range store.AllStatuses {
		label := "status: set " + string(s)
		if strings.Contains(strings.ToLower(label), q) {
			m.results = append(m.results, result{kind: "cmd", title: label, hint: "cmd"})
		}
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return itoa(n) + " " + one
	}
	return itoa(n) + " " + many
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	if neg {
		return "-" + out
	}
	return out
}

func shorten(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
