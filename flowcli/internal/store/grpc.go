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

	"google.golang.org/grpc"

	flowv1 "flowcli/internal/pb/flow/v1"
)

// GRPC is a Store that talks to the flowd core over gRPC. Reads are served
// from GetSnapshot; writes are unary mutations.
type GRPC struct {
	c   flowv1.FlowServiceClient
	who string

	mu     sync.Mutex
	cached *flowv1.GetSnapshotResponse // last fetched, shared by Nodes/Dependencies/Activity
}

// NewGRPC wraps an open gRPC connection (client side) as a Store. `who` is the
// author name attached to every write (humans and agents get the same byline).
func NewGRPC(conn *grpc.ClientConn, who string) *GRPC {
	return NewGRPCWithClient(flowv1.NewFlowServiceClient(conn), who)
}

// NewGRPCWithClient builds a Store backed by a supplied FlowServiceClient.
// This is the seam tests use to inject a fake/mock client; NewGRPC is a thin
// wrapper that constructs the real client from a connection.
func NewGRPCWithClient(client flowv1.FlowServiceClient, who string) *GRPC {
	return &GRPC{c: client, who: who}
}

func (g *GRPC) Projects(ctx context.Context) ([]Project, error) {
	res, err := g.c.ListProjects(ctx, &flowv1.ListProjectsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(res.Projects))
	for _, p := range res.Projects {
		out = append(out, Project{ID: p.Id, Name: p.Name, Description: p.Description})
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
