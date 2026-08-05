package store

import "context"

// NodeType mirrors the `type` column of the nodes table.
type NodeType string

const (
	TypeWorkPackage NodeType = "WORK_PACKAGE"
	TypeTask        NodeType = "TASK"
	TypeStep        NodeType = "STEP"
)

// Status mirrors the `status` column.
type Status string

const (
	StatusReady    Status = "READY"
	StatusBlocked  Status = "BLOCKED"
	StatusDeferred Status = "DEFERRED"
	StatusDone     Status = "DONE"
)

// AllStatuses is the order the status picker shows.
var AllStatuses = []Status{StatusReady, StatusBlocked, StatusDeferred, StatusDone}

// WPState is the explicit work-package lifecycle field added on top of the
// v1.0 data model, so the UI can decide what to expand and what to hide.
type WPState string

const (
	StatePlanned  WPState = "PLANNED"
	StateActive   WPState = "ACTIVE"
	StateDone     WPState = "DONE"
	StateArchived WPState = "ARCHIVED"
)

// VerifyResult is the cached outcome of running a node's condition.
type VerifyResult string

const (
	VerifyPass  VerifyResult = "pass"
	VerifyFail  VerifyResult = "fail"
	VerifyStale VerifyResult = "stale"
	VerifyNone  VerifyResult = "none"
)

type Project struct {
	ID          string
	Name        string
	Description string
	CreatedAt   int64
}

type Node struct {
	ID          string
	ProjectID   string
	ParentID    string
	Type        NodeType
	Title       string
	Description string
	Status      Status
	Condition   string

	// WORK_PACKAGE only.
	State WPState

	// Cached verification of Condition.
	LastResult VerifyResult
	LastRun    string
}

type Dependency struct {
	BlockerID string
	BlockedID string
}

// Store is the seam between the TUI and the engine. The in-memory
// implementation in this package is the prototype's fixture; a named-pipe or
// gRPC client implements the same five methods later and nothing in internal/ui
// has to change.
type Store interface {
	Projects(ctx context.Context) ([]Project, error)
	Nodes(ctx context.Context, projectID string) ([]Node, error)
	Dependencies(ctx context.Context, projectID string) ([]Dependency, error)

	// SetStatus writes one node's status. In the real engine this triggers the
	// downstream cascade; the prototype writes only the node you named.
	SetStatus(ctx context.Context, nodeID string, s Status) error

	// Verify runs the node's condition and returns the outcome.
	Verify(ctx context.Context, nodeID string) (VerifyResult, error)
}
