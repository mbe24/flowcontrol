package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	flowv1 "flowcli/internal/pb/flow/v1"
)

// --- status mapping (toDeclared) ---------------------------------------------

// The engine only accepts OPEN/DEFERRED/DONE. READY and BLOCKED both collapse
// to OPEN (the engine computes which one applies based on blockers). This is
// the subtle business logic Layer B pins down.
func TestToDeclaredMapping(t *testing.T) {
	cases := []struct {
		in   Status
		want flowv1.DeclaredStatus
	}{
		{Ready, flowv1.DeclaredStatus_DECLARED_STATUS_OPEN},
		{Blocked, flowv1.DeclaredStatus_DECLARED_STATUS_OPEN},
		{Deferred, flowv1.DeclaredStatus_DECLARED_STATUS_DEFERRED},
		{Done, flowv1.DeclaredStatus_DECLARED_STATUS_DONE},
	}
	for _, c := range cases {
		got, err := toDeclared(c.in)
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "mapping %s", c.in)
	}

	// an unknown status is rejected, not silently mapped
	_, err := toDeclared(Status("BOGUS"))
	assert.Error(t, err)
}

// --- SetStatus RPC -----------------------------------------------------------

func TestGRPCSetStatusRPC(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	// First call: a write must be issued with the author metadata and the node
	// id and the mapped declared status.
	mc.On("SetStatus", mock.Anything, mock.MatchedBy(func(req *flowv1.SetStatusRequest) bool {
		return req.NodeId == "T-1042" &&
			req.DeclaredStatus == flowv1.DeclaredStatus_DECLARED_STATUS_DONE &&
			req.Meta != nil && req.Meta.Author == "mbe"
	})).Return(&flowv1.SetStatusResponse{}, nil).Once()

	err := g.SetStatus(context.Background(), "T-1042", Done)
	require.NoError(t, err)
	mc.AssertExpectations(t)
}

// SetStatus must map Ready → OPEN (the engine decides ready vs blocked).
func TestGRPCSetStatusReadyGoesOpen(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	mc.On("SetStatus", mock.Anything, mock.MatchedBy(func(req *flowv1.SetStatusRequest) bool {
		return req.NodeId == "T-1099" &&
			req.DeclaredStatus == flowv1.DeclaredStatus_DECLARED_STATUS_OPEN
	})).Return(&flowv1.SetStatusResponse{}, nil).Once()

	require.NoError(t, g.SetStatus(context.Background(), "T-1099", Ready))
	mc.AssertExpectations(t)
}

// --- SetVerdict RPC ----------------------------------------------------------

func TestGRPCSetVerdictRPC(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "agent-7")

	mc.On("SetVerdict", mock.Anything, mock.MatchedBy(func(req *flowv1.SetVerdictRequest) bool {
		return req.NodeId == "T-1042" &&
			req.Verdict == flowv1.HumanVerdict_HUMAN_VERDICT_ACCEPTED &&
			req.Meta != nil && req.Meta.Author == "agent-7"
	})).Return(&flowv1.SetVerdictResponse{}, nil).Once()

	require.NoError(t, g.SetVerdict(context.Background(), "T-1042", Accepted))
	mc.AssertExpectations(t)
}

func TestGRPCSetVerdictClears(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	mc.On("SetVerdict", mock.Anything, mock.MatchedBy(func(req *flowv1.SetVerdictRequest) bool {
		return req.NodeId == "T-1042" &&
			req.Verdict == flowv1.HumanVerdict_HUMAN_VERDICT_UNSPECIFIED
	})).Return(&flowv1.SetVerdictResponse{}, nil).Once()

	require.NoError(t, g.SetVerdict(context.Background(), "T-1042", NoVerdict))
	mc.AssertExpectations(t)
}

// --- AddComment RPC ----------------------------------------------------------

func TestGRPCAddCommentRPC(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	mc.On("AddComment", mock.Anything, mock.MatchedBy(func(req *flowv1.AddCommentRequest) bool {
		return req.NodeId == "T-1042" &&
			req.Text == "rotate the token" &&
			req.Meta != nil && req.Meta.Author == "mbe"
	})).Return(&flowv1.AddCommentResponse{}, nil).Once()

	require.NoError(t, g.AddComment(context.Background(), "T-1042", "rotate the token"))
	mc.AssertExpectations(t)
}

// --- cache invalidation ------------------------------------------------------

// A successful write must clear the cached snapshot so the next read re-fetches
// from the server and reflects the mutation.
func TestGRPCWriteInvalidatesCache(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	first := &flowv1.GetSnapshotResponse{Nodes: []*flowv1.Node{{Id: "T-1042"}}}
	mc.On("GetSnapshot", mock.Anything, mock.Anything).Return(first, nil).Once()

	// seed the cache
	_, err := g.Nodes(context.Background(), "prj-travel")
	require.NoError(t, err)
	require.NotNil(t, g.cached, "expected cache seeded after first read")

	// a write invalidates
	mc.On("SetStatus", mock.Anything, mock.Anything).Return(&flowv1.SetStatusResponse{}, nil).Once()
	require.NoError(t, g.SetStatus(context.Background(), "T-1042", Done))
	assert.Nil(t, g.cached, "write must invalidate the cached snapshot")

	// next read re-fetches
	second := &flowv1.GetSnapshotResponse{Nodes: []*flowv1.Node{{Id: "T-1042"}, {Id: "T-1043"}}}
	mc.On("GetSnapshot", mock.Anything, mock.Anything).Return(second, nil).Once()
	nodes, err := g.Nodes(context.Background(), "prj-travel")
	require.NoError(t, err)
	assert.Len(t, nodes, 2, "after invalidation the read must re-fetch")

	mc.AssertExpectations(t)
}

// A failed write must NOT invalidate the (still valid) cache.
func TestGRPCWriteErrorKeepsCache(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	first := &flowv1.GetSnapshotResponse{Nodes: []*flowv1.Node{{Id: "T-1042"}}}
	mc.On("GetSnapshot", mock.Anything, mock.Anything).Return(first, nil).Once()
	_, err := g.Nodes(context.Background(), "prj-travel")
	require.NoError(t, err)
	require.NotNil(t, g.cached)

	// writing error → do not invalidate
	mc.On("SetStatus", mock.Anything, mock.Anything).Return((*flowv1.SetStatusResponse)(nil), assert.AnError).Once()
	err = g.SetStatus(context.Background(), "T-1042", Done)
	require.Error(t, err)
	assert.NotNil(t, g.cached, "failed write must leave cache intact")

	mc.AssertExpectations(t)
}

// --- read mapping sanity (GetSnapshot → Nodes) -------------------------------

// A snapshot's nodes translate to store.Nodes (this pins the read path end to
// end against the RPC seam, complementing the write-path tests above).
func TestGRPCReadsSnapshot(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	snap := &flowv1.GetSnapshotResponse{
		Nodes: []*flowv1.Node{
			{Id: "T-1042", ProjectId: "prj-travel", Kind: flowv1.NodeKind_NODE_KIND_TASK, Title: "Device-code",
				Status: flowv1.EffectiveStatus_EFFECTIVE_STATUS_READY, Condition: "pnpm test:auth"},
		},
		Dependencies: []*flowv1.Dependency{{BlockerId: "T-1041", BlockedId: "T-1042"}},
	}
	mc.On("GetSnapshot", mock.Anything, &flowv1.GetSnapshotRequest{ProjectId: "prj-travel"}).Return(snap, nil).Once()

	nodes, err := g.Nodes(context.Background(), "prj-travel")
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "T-1042", nodes[0].ID)
	assert.Equal(t, Task, nodes[0].Type)
	assert.Equal(t, Ready, nodes[0].Status)

	deps, err := g.Dependencies(context.Background(), "prj-travel")
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, "T-1041", deps[0].BlockerID)
	mc.AssertExpectations(t)
}

func TestGRPCCreateNodeRPC(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	res := &flowv1.CreateNodeResponse{
		Mutation: &flowv1.Mutation{ChangedNodes: []*flowv1.Node{{Id: "T-3000"}}},
	}
	mc.On("CreateNode", mock.Anything, mock.MatchedBy(func(req *flowv1.CreateNodeRequest) bool {
		return req.ProjectId == "prj-travel" &&
			req.ParentId == "T-1042" &&
			req.Kind == flowv1.NodeKind_NODE_KIND_STEP &&
			req.Title == "write the test" &&
			req.Condition == "go test" &&
			req.Meta != nil && req.Meta.Author == "mbe"
	})).Return(res, nil).Once()

	id, err := g.CreateNode(context.Background(), NewNode{
		ProjectID: "prj-travel",
		ParentID:  "T-1042",
		Type:      Step,
		Title:     "write the test",
		Condition: "go test",
	})
	require.NoError(t, err)
	assert.Equal(t, "T-3000", id)
	mc.AssertExpectations(t)
}

func TestGRPCCreateProjectUnsupported(t *testing.T) {
	mc := new(mockClient)
	g := NewGRPCWithClient(mc, "mbe")

	_, err := g.CreateProject(context.Background(), "New App", "desc")
	require.Error(t, err)
}
