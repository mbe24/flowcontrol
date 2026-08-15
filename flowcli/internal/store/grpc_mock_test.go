package store

import (
	"context"

	"github.com/stretchr/testify/mock"

	flowv1 "flowcli/internal/pb/flow/v1"
)

// mockClient is a Testify mock of the flowClient seam. Layer B tests inject it
// into NewGRPCWithClient and assert that each store write maps to the correct RPC
// with the correct payload. Read RPCs (GetSnapshot, ListProjects) are stubbed with
// `.Maybe()` or `.Return(...)` as each test needs. Only the unary RPCs the TUI
// uses are here — flowClient is that subset.
type mockClient struct {
	mock.Mock
}

var _ flowClient = (*mockClient)(nil)

func (m *mockClient) ListProjects(ctx context.Context, in *flowv1.ListProjectsRequest) (*flowv1.ListProjectsResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.ListProjectsResponse), args.Error(1)
}

func (m *mockClient) GetSnapshot(ctx context.Context, in *flowv1.GetSnapshotRequest) (*flowv1.GetSnapshotResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.GetSnapshotResponse), args.Error(1)
}

func (m *mockClient) CreateNode(ctx context.Context, in *flowv1.CreateNodeRequest) (*flowv1.CreateNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.CreateNodeResponse), args.Error(1)
}

func (m *mockClient) UpdateNode(ctx context.Context, in *flowv1.UpdateNodeRequest) (*flowv1.UpdateNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.UpdateNodeResponse), args.Error(1)
}

func (m *mockClient) DeleteNode(ctx context.Context, in *flowv1.DeleteNodeRequest) (*flowv1.DeleteNodeResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.DeleteNodeResponse), args.Error(1)
}

func (m *mockClient) SetStatus(ctx context.Context, in *flowv1.SetStatusRequest) (*flowv1.SetStatusResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.SetStatusResponse), args.Error(1)
}

func (m *mockClient) SetVerdict(ctx context.Context, in *flowv1.SetVerdictRequest) (*flowv1.SetVerdictResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.SetVerdictResponse), args.Error(1)
}

func (m *mockClient) AddComment(ctx context.Context, in *flowv1.AddCommentRequest) (*flowv1.AddCommentResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.AddCommentResponse), args.Error(1)
}

func (m *mockClient) AddDependency(ctx context.Context, in *flowv1.AddDependencyRequest) (*flowv1.AddDependencyResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.AddDependencyResponse), args.Error(1)
}

func (m *mockClient) RemoveDependency(ctx context.Context, in *flowv1.RemoveDependencyRequest) (*flowv1.RemoveDependencyResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.RemoveDependencyResponse), args.Error(1)
}

func (m *mockClient) CreateProject(ctx context.Context, in *flowv1.CreateProjectRequest) (*flowv1.CreateProjectResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*flowv1.CreateProjectResponse), args.Error(1)
}
