// Package store gRPC client — a Store backed by the flowd core over gRPC.
//
// Implements the same Store seam as Memory, so nothing under internal/ui
// changes when the CLI talks to the real engine. The proto contract in
// ../pb/flow/v1 is the single source of truth; the mapping here translates the
// engine's two-status model (Declared vs Effective) into the TUI's flat Status.
package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	flowv1 "flowcli/internal/pb/flow/v1"
)

// flowClient is the subset of FlowService the TUI actually uses — all unary.
// It is deliberately local, not grpc-go's or connect-go's generated interface, so
// the store (and its test mock) depend on neither transport's request/response
// wrappers nor the server-streaming Watch. The connect-go transport is adapted to
// it in connect.go.
type flowClient interface {
	ListProjects(context.Context, *flowv1.ListProjectsRequest) (*flowv1.ListProjectsResponse, error)
	GetSnapshot(context.Context, *flowv1.GetSnapshotRequest) (*flowv1.GetSnapshotResponse, error)
	SetStatus(context.Context, *flowv1.SetStatusRequest) (*flowv1.SetStatusResponse, error)
	SetVerdict(context.Context, *flowv1.SetVerdictRequest) (*flowv1.SetVerdictResponse, error)
	AddComment(context.Context, *flowv1.AddCommentRequest) (*flowv1.AddCommentResponse, error)
	CreateProject(context.Context, *flowv1.CreateProjectRequest) (*flowv1.CreateProjectResponse, error)
	CreateNode(context.Context, *flowv1.CreateNodeRequest) (*flowv1.CreateNodeResponse, error)
	UpdateNode(context.Context, *flowv1.UpdateNodeRequest) (*flowv1.UpdateNodeResponse, error)
	DeleteNode(context.Context, *flowv1.DeleteNodeRequest) (*flowv1.DeleteNodeResponse, error)
	AddDependency(context.Context, *flowv1.AddDependencyRequest) (*flowv1.AddDependencyResponse, error)
	RemoveDependency(context.Context, *flowv1.RemoveDependencyRequest) (*flowv1.RemoveDependencyResponse, error)
}

// GRPC is a Store that talks to a daemon over the flowClient seam. Reads are
// served from GetSnapshot; writes are unary mutations. (Named GRPC for history;
// the transport is now gRPC-web over HTTP/1.1 — see connect.go / NewGRPC.)
type GRPC struct {
	c   flowClient
	who string

	mu     sync.Mutex
	cached *flowv1.GetSnapshotResponse // last fetched, shared by Nodes/Dependencies/Activity
}

// NewGRPCWithClient builds a Store backed by a supplied flowClient. This is the
// seam tests use to inject a fake/mock client; NewGRPC (connect.go) is the thin
// wrapper that constructs the real gRPC-web client.
func NewGRPCWithClient(client flowClient, who string) *GRPC {
	return &GRPC{c: client, who: who}
}

func (g *GRPC) Projects(ctx context.Context) ([]Project, error) {
	res, err := g.c.ListProjects(ctx, &flowv1.ListProjectsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(res.Projects))
	for _, p := range res.Projects {
		proj := Project{ID: p.Id, Name: p.Name, Description: p.Description}
		// ListProjects carries no rollup, so fold the per-work-package Progress
		// the engine computes in GetSnapshot into a project-level done/total —
		// otherwise the landing shows 0/0. One snapshot per project is fine for
		// the landing list (small, shown once). Fetched directly rather than via
		// g.snapshot so listing never evicts the open board's cached snapshot.
		snap, err := g.c.GetSnapshot(ctx, &flowv1.GetSnapshotRequest{ProjectId: p.Id})
		if err != nil {
			return nil, err
		}
		for _, pr := range snap.Progress {
			proj.Progress.Done += int(pr.Done)
			proj.Progress.Total += int(pr.Total)
		}
		out = append(out, proj)
	}
	return out, nil
}

// snapshot returns a cached GetSnapshot for the project, fetching it only on
// the first read. The TUI's refresh calls Nodes/Dependencies/Activity back to
// back; they share one round trip and one consistent view. Writes clear the
// cache so the next refresh re-fetches.
func (g *GRPC) snapshot(ctx context.Context, projectID string) (*flowv1.GetSnapshotResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cached != nil {
		return g.cached, nil
	}
	res, err := g.c.GetSnapshot(ctx, &flowv1.GetSnapshotRequest{ProjectId: projectID})
	if err != nil {
		return nil, err
	}
	g.cached = res
	return res, nil
}

// invalidate drops the cached snapshot after a write so the next read reflects
// the mutation.
func (g *GRPC) invalidate() {
	g.mu.Lock()
	g.cached = nil
	g.mu.Unlock()
}

func (g *GRPC) Nodes(ctx context.Context, projectID string) ([]Node, error) {
	res, err := g.snapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(res.Nodes))
	for _, n := range res.Nodes {
		out = append(out, fromProtoNode(n))
	}
	return out, nil
}

func (g *GRPC) Dependencies(ctx context.Context, projectID string) ([]Dependency, error) {
	res, err := g.snapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Dependency, 0, len(res.Dependencies))
	for _, d := range res.Dependencies {
		out = append(out, Dependency{BlockerID: d.BlockerId, BlockedID: d.BlockedId})
	}
	return out, nil
}

func (g *GRPC) Activity(ctx context.Context, projectID string) ([]ActivityEntry, error) {
	res, err := g.snapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityEntry, 0, len(res.RecentEvents))
	for _, e := range res.RecentEvents {
		out = append(out, fromProtoEvent(e))
	}
	return out, nil
}

func (g *GRPC) SetStatus(ctx context.Context, nodeID string, status Status) error {
	declared, err := toDeclared(status)
	if err != nil {
		return err
	}
	_, err = g.c.SetStatus(ctx, &flowv1.SetStatusRequest{
		Meta:           &flowv1.WriteMeta{Author: g.who},
		NodeId:         nodeID,
		DeclaredStatus: declared,
	})
	if err == nil {
		g.invalidate()
	}
	return err
}

func (g *GRPC) SetVerdict(ctx context.Context, nodeID string, verdict HumanVerdict) error {
	v, err := toVerdict(verdict)
	if err != nil {
		return err
	}
	_, err = g.c.SetVerdict(ctx, &flowv1.SetVerdictRequest{
		Meta:    &flowv1.WriteMeta{Author: g.who},
		NodeId:  nodeID,
		Verdict: v,
	})
	if err == nil {
		g.invalidate()
	}
	return err
}

func (g *GRPC) AddComment(ctx context.Context, nodeID, text string) error {
	_, err := g.c.AddComment(ctx, &flowv1.AddCommentRequest{
		Meta:   &flowv1.WriteMeta{Author: g.who},
		NodeId: nodeID,
		Text:   text,
	})
	if err == nil {
		g.invalidate()
	}
	return err
}

func (g *GRPC) CreateProject(ctx context.Context, name, description string, seed bool) (string, error) {
	res, err := g.c.CreateProject(ctx, &flowv1.CreateProjectRequest{
		Meta:        &flowv1.WriteMeta{Author: g.who},
		Name:        name,
		Description: description,
	})
	if err != nil {
		return "", err
	}
	g.invalidate()
	id := res.GetProject().GetId()
	// Seeding is client-side composition: create the project's first work package.
	if id != "" && seed {
		if _, err := g.CreateNode(ctx, NewNode{ProjectID: id, Type: WorkPackage, Title: name}); err != nil {
			return id, err
		}
	}
	return id, nil
}

func (g *GRPC) CreateNode(ctx context.Context, n NewNode) (string, error) {
	res, err := g.c.CreateNode(ctx, &flowv1.CreateNodeRequest{
		Meta:        &flowv1.WriteMeta{Author: g.who},
		ProjectId:   n.ProjectID,
		ParentId:    n.ParentID,
		Kind:        toNodeKind(n.Type),
		Title:       n.Title,
		Description: strings.Join(n.Description, "\n"),
		Condition:   n.Condition,
	})
	if err != nil {
		return "", err
	}
	g.invalidate()
	// The created node's id comes back in the mutation's changed set.
	if res != nil && len(res.Mutation.GetChangedNodes()) > 0 {
		return res.Mutation.GetChangedNodes()[0].Id, nil
	}
	return "", nil
}

func (g *GRPC) UpdateNode(ctx context.Context, nodeID string, updates NodeUpdate) error {
	req := &flowv1.UpdateNodeRequest{
		Meta:   &flowv1.WriteMeta{Author: g.who},
		NodeId: nodeID,
	}
	if updates.Title != nil {
		req.UpdateMask = append(req.UpdateMask, "title")
		req.Title = *updates.Title
	}
	if updates.Condition != nil {
		req.UpdateMask = append(req.UpdateMask, "condition")
		req.Condition = *updates.Condition
	}
	_, err := g.c.UpdateNode(ctx, req)
	if err == nil {
		g.invalidate()
	}
	return err
}

func (g *GRPC) DeleteNode(ctx context.Context, nodeID string) error {
	_, err := g.c.DeleteNode(ctx, &flowv1.DeleteNodeRequest{
		Meta:   &flowv1.WriteMeta{Author: g.who},
		NodeId: nodeID,
	})
	if err == nil {
		g.invalidate()
	}
	return err
}

func (g *GRPC) AddDependency(ctx context.Context, blockerID, blockedID string) error {
	_, err := g.c.AddDependency(ctx, &flowv1.AddDependencyRequest{
		Meta:      &flowv1.WriteMeta{Author: g.who},
		BlockerId: blockerID,
		BlockedId: blockedID,
	})
	if err == nil {
		g.invalidate()
	}
	return err
}

func (g *GRPC) RemoveDependency(ctx context.Context, blockerID, blockedID string) error {
	_, err := g.c.RemoveDependency(ctx, &flowv1.RemoveDependencyRequest{
		Meta:      &flowv1.WriteMeta{Author: g.who},
		BlockerId: blockerID,
		BlockedId: blockedID,
	})
	if err == nil {
		g.invalidate()
	}
	return err
}

func toNodeKind(t NodeType) flowv1.NodeKind {
	switch t {
	case WorkPackage:
		return flowv1.NodeKind_NODE_KIND_WORK_PACKAGE
	case Task:
		return flowv1.NodeKind_NODE_KIND_TASK
	default:
		return flowv1.NodeKind_NODE_KIND_STEP
	}
}

// ─── mapping ────────────────────────────────────────────────────────────────

func fromProtoNode(n *flowv1.Node) Node {
	return Node{
		ID:           n.Id,
		ProjectID:    n.ProjectId,
		ParentID:     n.ParentId,
		Type:         fromNodeKind(n.Kind),
		Title:        n.Title,
		Description:  splitParagraphs(n.Description),
		Status:       fromEffective(n.Status),
		Condition:    n.Condition,
		Note:         n.Note,
		State:        fromWPState(n.WpState),
		Verification: fromVerification(n.Verification),
	}
}

func fromNodeKind(k flowv1.NodeKind) NodeType {
	switch k {
	case flowv1.NodeKind_NODE_KIND_WORK_PACKAGE:
		return WorkPackage
	case flowv1.NodeKind_NODE_KIND_STEP:
		return Step
	default:
		return Task
	}
}

func fromEffective(s flowv1.EffectiveStatus) Status {
	switch s {
	case flowv1.EffectiveStatus_EFFECTIVE_STATUS_READY:
		return Ready
	case flowv1.EffectiveStatus_EFFECTIVE_STATUS_BLOCKED:
		return Blocked
	case flowv1.EffectiveStatus_EFFECTIVE_STATUS_DEFERRED:
		return Deferred
	case flowv1.EffectiveStatus_EFFECTIVE_STATUS_DONE:
		return Done
	default:
		return Ready
	}
}

func fromWPState(s flowv1.WorkPackageState) WPState {
	switch s {
	case flowv1.WorkPackageState_WORK_PACKAGE_STATE_PLANNED:
		return Planned
	case flowv1.WorkPackageState_WORK_PACKAGE_STATE_DONE:
		return WPDone
	case flowv1.WorkPackageState_WORK_PACKAGE_STATE_ARCHIVED:
		return Archived
	default:
		return Active
	}
}

func fromVerification(v *flowv1.Verification) Verification {
	if v == nil {
		return Verification{Agent: NoReport, Human: NoVerdict}
	}
	agent := NoReport
	switch {
	case v.Stale:
		agent = Stale
	case v.AgentResult == flowv1.AgentResult_AGENT_RESULT_PASS:
		agent = Pass
	case v.AgentResult == flowv1.AgentResult_AGENT_RESULT_FAIL:
		agent = Fail
	}
	human := NoVerdict
	switch v.HumanVerdict {
	case flowv1.HumanVerdict_HUMAN_VERDICT_ACCEPTED:
		human = Accepted
	case flowv1.HumanVerdict_HUMAN_VERDICT_REJECTED:
		human = Rejected
	}
	return Verification{
		Agent:     agent,
		AgentName: v.AgentName,
		AgentWhen: when(v.AgentAt),
		Human:     human,
		HumanWhen: when(v.HumanAt),
	}
}

func fromProtoEvent(e *flowv1.Event) ActivityEntry {
	return ActivityEntry{
		ID:     fmt.Sprint(e.Seq),
		NodeID: e.NodeId,
		Kind:   fromEventKind(e.Kind),
		Author: e.Author,
		When:   when(e.CreatedAt),
		Text:   e.Summary,
	}
}

func fromEventKind(k flowv1.EventKind) ActivityKind {
	switch k {
	case flowv1.EventKind_EVENT_KIND_STATUS_SET:
		return ActStatus
	case flowv1.EventKind_EVENT_KIND_AGENT_REPORTED, flowv1.EventKind_EVENT_KIND_VERDICT_SET:
		return ActVerify
	case flowv1.EventKind_EVENT_KIND_COMMENT:
		return ActComment
	case flowv1.EventKind_EVENT_KIND_NODE_UPDATED:
		return ActEdit
	default:
		return ActEdit
	}
}

// toDeclared maps the TUI's flat Status onto what the engine can be asked to
// set. READY and BLOCKED are not settable — they are the engine's answer — so
// both collapse to OPEN (open + no blockers = ready; open + a blocker = the
// engine computes blocked).
func toDeclared(s Status) (flowv1.DeclaredStatus, error) {
	switch s {
	case Ready, Blocked:
		return flowv1.DeclaredStatus_DECLARED_STATUS_OPEN, nil
	case Deferred:
		return flowv1.DeclaredStatus_DECLARED_STATUS_DEFERRED, nil
	case Done:
		return flowv1.DeclaredStatus_DECLARED_STATUS_DONE, nil
	}
	return flowv1.DeclaredStatus_DECLARED_STATUS_UNSPECIFIED, fmt.Errorf("cannot set status %q on the core", s)
}

func toVerdict(v HumanVerdict) (flowv1.HumanVerdict, error) {
	switch v {
	case Accepted:
		return flowv1.HumanVerdict_HUMAN_VERDICT_ACCEPTED, nil
	case Rejected:
		return flowv1.HumanVerdict_HUMAN_VERDICT_REJECTED, nil
	case NoVerdict:
		return flowv1.HumanVerdict_HUMAN_VERDICT_UNSPECIFIED, nil
	}
	return flowv1.HumanVerdict_HUMAN_VERDICT_UNSPECIFIED, fmt.Errorf("unknown verdict %q", v)
}

// splitParagraphs turns a markdown body (one string) into the TUI's paragraph
// slice, splitting on blank lines.
func splitParagraphs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// when renders a unix timestamp as the compact relative string the UI expects
// ("just now", "5m ago", "3d ago"). 0 means "never" and renders as "".
func when(unix int64) string {
	if unix == 0 {
		return ""
	}
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	}
}
