package store

import (
	"context"

	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	flowv1 "flowcli/internal/pb/flow/v1"
)

// mockClient is a Testify mock of the flowv1.FlowServiceClient seam. Layer B
// tests inject it into NewGRPCWithClient and assert that each store write maps
// to the correct flowd RPC with the correct payload. Read RPCs (GetSnapshot,
// ListProjects) are stubbed with `.Maybe()` or `.Return(...)` as each test needs.
type mockClient struct {
	mock.Mock
}

var _ flowv1.FlowServiceClient = (*mockClient)(nil)

func (m *mockClient) ListProjects(ctx context.Context, in *flowv1.ListProjectsRequest, opts ...grpc.CallOption) (*flowv1.ListProjectsResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.ListProjectsResponse), args.Error(1)
}

func (m *mockClient) GetSnapshot(ctx context.Context, in *flowv1.GetSnapshotRequest, opts ...grpc.CallOption) (*flowv1.GetSnapshotResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.GetSnapshotResponse), args.Error(1)
}

func (m *mockClient) ListEvents(ctx context.Context, in *flowv1.ListEventsRequest, opts ...grpc.CallOption) (*flowv1.ListEventsResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.ListEventsResponse), args.Error(1)
}

func (m *mockClient) Search(ctx context.Context, in *flowv1.SearchRequest, opts ...grpc.CallOption) (*flowv1.SearchResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.SearchResponse), args.Error(1)
}

func (m *mockClient) Watch(ctx context.Context, in *flowv1.WatchRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[flowv1.WatchResponse], error) {
	args := m.Called(ctx, in)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(grpc.ServerStreamingClient[flowv1.WatchResponse]), args.Error(1)
}

func (m *mockClient) CreateNode(ctx context.Context, in *flowv1.CreateNodeRequest, opts ...grpc.CallOption) (*flowv1.CreateNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.CreateNodeResponse), args.Error(1)
}

func (m *mockClient) UpdateNode(ctx context.Context, in *flowv1.UpdateNodeRequest, opts ...grpc.CallOption) (*flowv1.UpdateNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.UpdateNodeResponse), args.Error(1)
}

func (m *mockClient) DeleteNode(ctx context.Context, in *flowv1.DeleteNodeRequest, opts ...grpc.CallOption) (*flowv1.DeleteNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.DeleteNodeResponse), args.Error(1)
}

func (m *mockClient) SetStatus(ctx context.Context, in *flowv1.SetStatusRequest, opts ...grpc.CallOption) (*flowv1.SetStatusResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.SetStatusResponse), args.Error(1)
}

func (m *mockClient) ReportCondition(ctx context.Context, in *flowv1.ReportConditionRequest, opts ...grpc.CallOption) (*flowv1.ReportConditionResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.ReportConditionResponse), args.Error(1)
}

func (m *mockClient) SetVerdict(ctx context.Context, in *flowv1.SetVerdictRequest, opts ...grpc.CallOption) (*flowv1.SetVerdictResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.SetVerdictResponse), args.Error(1)
}

func (m *mockClient) AddComment(ctx context.Context, in *flowv1.AddCommentRequest, opts ...grpc.CallOption) (*flowv1.AddCommentResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.AddCommentResponse), args.Error(1)
}

func (m *mockClient) AddDependency(ctx context.Context, in *flowv1.AddDependencyRequest, opts ...grpc.CallOption) (*flowv1.AddDependencyResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.AddDependencyResponse), args.Error(1)
}

func (m *mockClient) RemoveDependency(ctx context.Context, in *flowv1.RemoveDependencyRequest, opts ...grpc.CallOption) (*flowv1.RemoveDependencyResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.RemoveDependencyResponse), args.Error(1)
}

func (m *mockClient) Undo(ctx context.Context, in *flowv1.UndoRequest, opts ...grpc.CallOption) (*flowv1.UndoResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.UndoResponse), args.Error(1)
}

func (m *mockClient) MoveNode(ctx context.Context, in *flowv1.MoveNodeRequest, opts ...grpc.CallOption) (*flowv1.MoveNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.MoveNodeResponse), args.Error(1)
}

func (m *mockClient) CreateProject(ctx context.Context, in *flowv1.CreateProjectRequest, opts ...grpc.CallOption) (*flowv1.CreateProjectResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.CreateProjectResponse), args.Error(1)
}

func (m *mockClient) UpdateProject(ctx context.Context, in *flowv1.UpdateProjectRequest, opts ...grpc.CallOption) (*flowv1.UpdateProjectResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.UpdateProjectResponse), args.Error(1)
}

func (m *mockClient) ArchiveProject(ctx context.Context, in *flowv1.ArchiveProjectRequest, opts ...grpc.CallOption) (*flowv1.ArchiveProjectResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.ArchiveProjectResponse), args.Error(1)
}

func (m *mockClient) PollChanges(ctx context.Context, in *flowv1.PollChangesRequest, opts ...grpc.CallOption) (*flowv1.PollChangesResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.PollChangesResponse), args.Error(1)
}
