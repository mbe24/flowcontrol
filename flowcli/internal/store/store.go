// Package store defines the seam between the TUI and the engine.
//
// The in-memory implementation lives beside this file; a client that talks to
// the Rust core over gRPC or a named pipe implements the same interface and
// nothing under internal/ui changes.
package store

import "context"

type NodeType string

const (
	WorkPackage NodeType = "WORK_PACKAGE"
	Task        NodeType = "TASK"
	Step        NodeType = "STEP"
)

type Status string

const (
	Ready    Status = "READY"
	Blocked  Status = "BLOCKED"
	Deferred Status = "DEFERRED"
	Done     Status = "DONE"
)

var AllStatuses = []Status{Ready, Blocked, Deferred, Done}

// WPState is an addition to datamodel.md v1.0: work-package lifecycle, set
// explicitly rather than derived from child status.
type WPState string

const (
	Planned  WPState = "PLANNED"
	Active   WPState = "ACTIVE"
	WPDone   WPState = "DONE"
	Archived WPState = "ARCHIVED"
)

// AgentResult is what an agent reported about a node's condition. fctrl never
// runs a condition itself.
type AgentResult string

const (
	Pass     AgentResult = "pass"
	Fail     AgentResult = "fail"
	Stale    AgentResult = "stale"
	NoReport AgentResult = "none"
)

// HumanVerdict is the operator's explicit acceptance, independent of the agent.
type HumanVerdict string

const (
	Accepted  HumanVerdict = "accepted"
	Rejected  HumanVerdict = "rejected"
	NoVerdict HumanVerdict = "none"
)

type Verification struct {
	Agent     AgentResult
	AgentName string
	AgentWhen string
	Human     HumanVerdict
	HumanWhen string
}

type Project struct {
	ID          string
	Name        string
	Description string
	// Progress reports how many leaf nodes are done out of total, as a
	// fraction the landing renders (e.g. 0/0 or 17/23). Populated by the
	// store; empty numbers render as 0/0.
	Progress struct {
		Done  int
		Total int
	}
	// Archived hides the project from the landing picker. Deleted projects are
	// not modelled yet — archiving is the reversible form of removal.
	Archived bool
}

// NewNode is the client-side payload for CreateNode. Condition is only used by
// Task and Step; Description only by WorkPackage and Task.
type NewNode struct {
	ProjectID   string
	ParentID    string
	Type        NodeType
	Title       string
	Description []string
	Condition   string
}

type Node struct {
	ID        string
	ProjectID string
	ParentID  string
	Type      NodeType
	Title     string
	// Paragraphs. Markdown later; plain text for now.
	Description []string
	Status      Status
	Condition   string
	// Step only: a few sentences, folded away until expanded.
	Note string
	// WorkPackage only.
	State WPState
	// Task and WorkPackage only. Steps show condition text but carry no flag.
	Verification Verification
}

type Dependency struct {
	BlockerID string
	BlockedID string
}

type ActivityKind string

const (
	ActStatus  ActivityKind = "status"
	ActVerify  ActivityKind = "verify"
	ActEdit    ActivityKind = "edit"
	ActComment ActivityKind = "comment"
)

type ActivityEntry struct {
	ID     string
	NodeID string
	Kind   ActivityKind
	// Plain name. Agents get no special badge — authorship is just a byline.
	Author string
	When   string
	Text   string
}

type Store interface {
	Projects(ctx context.Context) ([]Project, error)
	Nodes(ctx context.Context, projectID string) ([]Node, error)
	Dependencies(ctx context.Context, projectID string) ([]Dependency, error)
	Activity(ctx context.Context, projectID string) ([]ActivityEntry, error)

	// SetStatus writes one node. The engine owns the downstream cascade.
	SetStatus(ctx context.Context, nodeID string, status Status) error
	// SetVerdict records the operator's acceptance of a reported condition.
	SetVerdict(ctx context.Context, nodeID string, verdict HumanVerdict) error
	AddComment(ctx context.Context, nodeID, text string) error

	// CreateProject adds a project and returns its id. The memory store
	// assigns ids; the gRPC path will round-trip through the engine once the
	// proto gains CreateProject.
	CreateProject(ctx context.Context, name, description string, seed bool) (string, error)
	// CreateNode adds a child node and returns its id.
	CreateNode(ctx context.Context, n NewNode) (string, error)
	// UpdateNode applies the non-nil fields of updates (title / condition).
	// Editing a condition marks the agent report stale.
	UpdateNode(ctx context.Context, nodeID string, updates NodeUpdate) error
	// DeleteNode removes a node and (in the engine's model) its descendants
	// and edges. The TUI confirms collateral first.
	DeleteNode(ctx context.Context, nodeID string) error
	// AddDependency records that blocker must finish before blocked.
	AddDependency(ctx context.Context, blockerID, blockedID string) error
	// RemoveDependency drops one directed blocker → blocked edge.
	RemoveDependency(ctx context.Context, blockerID, blockedID string) error
}

// NodeUpdate carries the editable fields for UpdateNode. Nil fields are left
// untouched so a caller edits only what changed.
type NodeUpdate struct {
	Title     *string
	Condition *string
}

// Badge resolves an agent report and a human verdict into one display state.
// The human verdict always wins; the agent's report stays visible beside it.
type Badge struct {
	Glyph    string
	Label    string
	Detail   string
	Kind     Status // reused for colour: Ready=good, Blocked=bad, Deferred=unknown
	Accepted bool
}

func (v Verification) Badge() Badge {
	agent := ""
	if v.AgentName != "" {
		agent = v.AgentName + " · " + v.AgentWhen
	}
	switch {
	case v.Human == Accepted:
		label := "verified"
		if v.Agent == Fail {
			label = "accepted by you — agent reported failure"
		}
		return Badge{"√", label, agent, Ready, true}
	case v.Human == Rejected:
		return Badge{"×", "rejected by you", agent, Blocked, false}
	case v.Agent == Pass:
		return Badge{"√", "verified by agent", agent, Ready, false}
	case v.Agent == Fail:
		return Badge{"×", "agent reported a failure", agent, Blocked, false}
	case v.Agent == Stale:
		return Badge{"~", "report is out of date", agent, Deferred, false}
	}
	return Badge{"-", "not verified", "", Deferred, false}
}
