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
	Pass      AgentResult = "pass"
	Fail      AgentResult = "fail"
	Stale     AgentResult = "stale"
	NoReport  AgentResult = "none"
)

// HumanVerdict is the operator's explicit acceptance, independent of the agent.
type HumanVerdict string

const (
	Accepted   HumanVerdict = "accepted"
	Rejected   HumanVerdict = "rejected"
	NoVerdict  HumanVerdict = "none"
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
