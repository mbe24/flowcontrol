package ui

import (
	"context"
	"sort"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"

	"flowcli/internal/store"
)

type Screen int

const (
	ScreenLanding Screen = iota
	ScreenTree
	ScreenLanes
	ScreenChain
	ScreenDetail
	ScreenActivity
)

type Overlay int

const (
	OverlayNone Overlay = iota
	OverlayFinder
	OverlayProjects
	OverlayStatus
	OverlayConfirm
	OverlayComment
	OverlayHelp
	OverlayCreate
	OverlayCascade
)

// Lane layout thresholds, derived from the drawn card widths:
// four lanes = 4×22 + gutters + frame = 98; two = 2×30 + gutter + frame = 67.
const (
	FourLaneMin = 100
	TwoLaneMin  = 68
	OneLaneMin  = 44
)

// finderVisible is the number of result rows shown in the fixed-height find
// dialog; extra results are reached by scrolling.
const finderVisible = 6

// finderInner is the fixed content width of the find dialog. Every line is
// padded/truncated to this many cells so the dialog keeps a constant size
// regardless of how many or how long the results are.
const finderInner = 62

type loadedMsg struct {
	nodes    []store.Node
	deps     []store.Dependency
	activity []store.ActivityEntry
	projects []store.Project
	err      error
}

type refreshedMsg loadedMsg

// row is one visible line in the tree, flattened from the hierarchy.
type row struct {
	node     store.Node
	depth    int
	isWP     bool
	expanded bool
}

type Model struct {
	store store.Store
	ctx   context.Context

	width, height int

	projects  []store.Project
	projectID string
	nodes     []store.Node
	deps      []store.Dependency
	activity  []store.ActivityEntry

	byID     map[string]store.Node
	blockers map[string][]string
	blocks   map[string][]string

	screen  Screen
	overlay Overlay
	err     error
	flash   string

	help help.Model

	// prevScreen is the view we came from into an overlay detail, so ESC
	// returns to it instead of always falling back to the tree. Updated every
	// time we enter detail.
	prevScreen Screen

	// tree
	rows       []row
	cursor     int
	treeScroll int
	collapsed  map[string]bool
	showDone   bool

	// lanes
	lane       int
	laneCursor [4]int

	// chain
	chainRows   []chainRow
	chainCursor int
	chainWP     int
	focusID     string

	// detail
	selectedID   string
	stepCursor   int
	openSteps    map[string]bool
	descScroll   int
	activityScrl int

	// overlays
	input       textinput.Model
	finderHits  []store.Node
	finderIdx   int
	finderScroll int
	fromFinder  bool
	statusIdx   int
	projectIdx  int
	confirmID   string
	lastStatus  *struct {
		id   string
		prev store.Status
	}

	// create / landing (Phase C designer components); cascade (Phase D)
	create  createState
	landing landingState
}

func New(s store.Store) Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 200

	return Model{
		store:     s,
		ctx:       context.Background(),
		projectID: "prj-travel",
		screen:    ScreenLanding,
		collapsed: map[string]bool{},
		openSteps: map[string]bool{},
		input:     ti,
		help:      help.New(),
		width:     120,
		height:    40,
		landing:   landingState{},
		create:    createState{errAt: -1},
	}
}

func (m Model) Init() tea.Cmd { return m.load }

func (m Model) load() tea.Msg {
	projects, err := m.store.Projects(m.ctx)
	if err != nil {
		return loadedMsg{err: err}
	}
	nodes, err := m.store.Nodes(m.ctx, m.projectID)
	if err != nil {
		return loadedMsg{err: err}
	}
	deps, err := m.store.Dependencies(m.ctx, m.projectID)
	if err != nil {
		return loadedMsg{err: err}
	}
	act, err := m.store.Activity(m.ctx, m.projectID)
	if err != nil {
		return loadedMsg{err: err}
	}
	return loadedMsg{nodes: nodes, deps: deps, activity: act, projects: projects}
}

func (m Model) refresh() tea.Msg {
	msg := m.load().(loadedMsg)
	return refreshedMsg(msg)
}

// index rebuilds the id and dependency maps, then the flattened tree.
func (m *Model) index() {
	m.byID = make(map[string]store.Node, len(m.nodes))
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
	m.buildChain()
}

func (m *Model) workPackages() []store.Node {
	var out []store.Node
	for _, n := range m.nodes {
		if n.Type == store.WorkPackage {
			out = append(out, n)
		}
	}
	prio := func(s store.WPState) int {
		switch s {
		case store.Active:
			return 0
		case store.Planned:
			return 1
		case store.WPDone:
			return 2
		}
		return 3
	}
	sort.SliceStable(out, func(i, j int) bool { return prio(out[i].State) < prio(out[j].State) })
	return out
}

func (m *Model) childrenOf(parent string, t store.NodeType) []store.Node {
	var out []store.Node
	for _, n := range m.nodes {
		if n.ParentID == parent && n.Type == t {
			out = append(out, n)
		}
	}
	return out
}

// buildRows flattens work packages and their tasks into the visible tree.
// Done packages are folded away behind a disclosure row.
func (m *Model) buildRows() {
	m.rows = nil
	for _, wp := range m.workPackages() {
		if (wp.State == store.WPDone || wp.State == store.Archived) && !m.showDone {
			continue
		}
		expanded := !m.collapsed[wp.ID]
		m.rows = append(m.rows, row{node: wp, isWP: true, expanded: expanded})
		if !expanded {
			continue
		}
		for _, t := range m.childrenOf(wp.ID, store.Task) {
			m.rows = append(m.rows, row{node: t, depth: 1})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m *Model) current() (store.Node, bool) {
	switch m.screen {
	case ScreenLanes:
		cards := m.laneTasks(m.laneSet()[m.lane])
		if len(cards) == 0 {
			return store.Node{}, false
		}
		i := min(m.laneCursor[m.lane], len(cards)-1)
		return cards[i], true
	case ScreenChain:
		if len(m.chainRows) == 0 {
			return store.Node{}, false
		}
		i := min(m.chainCursor, len(m.chainRows)-1)
		return m.chainRows[i].node, true
	case ScreenDetail, ScreenActivity:
		n, ok := m.byID[m.selectedID]
		return n, ok
	}
	if len(m.rows) == 0 {
		return store.Node{}, false
	}
	return m.rows[m.cursor].node, true
}

func (m *Model) stepsOf(taskID string) []store.Node { return m.childrenOf(taskID, store.Step) }

func (m *Model) stepRatio(taskID string) (int, int) {
	steps := m.stepsOf(taskID)
	done := 0
	for _, s := range steps {
		if s.Status == store.Done {
			done++
		}
	}
	return done, len(steps)
}

// counts returns done/ready/blocked/deferred leaf counts under a work package.
func (m *Model) counts(wpID string) (d, r, b, df, total int) {
	for _, t := range m.childrenOf(wpID, store.Task) {
		leaves := m.stepsOf(t.ID)
		if len(leaves) == 0 {
			leaves = []store.Node{t}
		}
		for _, l := range leaves {
			total++
			switch l.Status {
			case store.Done:
				d++
			case store.Ready:
				r++
			case store.Blocked:
				b++
			default:
				df++
			}
		}
	}
	return
}

func (m *Model) hueOf(wpID string) int {
	for i, wp := range m.workPackages() {
		if wp.ID == wpID {
			return i
		}
	}
	return 0
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
