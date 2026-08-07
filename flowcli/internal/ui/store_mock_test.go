package ui

import (
	"context"

	"github.com/stretchr/testify/mock"

	"flowcli/internal/store"
)

// mockStore is a Testify mock of the store.Store seam. Layer A tests inject it
// and assert that pressing keys in the TUI dispatches the correct store write
// (SetStatus / SetVerdict / AddComment) rather than reading one. Reads are not
// exercised here — callers set up expectations for whatever the test drives.
type mockStore struct {
	mock.Mock
}

var _ store.Store = (*mockStore)(nil)

func (m *mockStore) Projects(ctx context.Context) ([]store.Project, error) {
	args := m.Called(ctx)
	return args.Get(0).([]store.Project), args.Error(1)
}

func (m *mockStore) Nodes(ctx context.Context, projectID string) ([]store.Node, error) {
	args := m.Called(ctx, projectID)
	return args.Get(0).([]store.Node), args.Error(1)
}

func (m *mockStore) Dependencies(ctx context.Context, projectID string) ([]store.Dependency, error) {
	args := m.Called(ctx, projectID)
	return args.Get(0).([]store.Dependency), args.Error(1)
}

func (m *mockStore) Activity(ctx context.Context, projectID string) ([]store.ActivityEntry, error) {
	args := m.Called(ctx, projectID)
	return args.Get(0).([]store.ActivityEntry), args.Error(1)
}

func (m *mockStore) SetStatus(ctx context.Context, nodeID string, status store.Status) error {
	args := m.Called(ctx, nodeID, status)
	return args.Error(0)
}

func (m *mockStore) SetVerdict(ctx context.Context, nodeID string, verdict store.HumanVerdict) error {
	args := m.Called(ctx, nodeID, verdict)
	return args.Error(0)
}

func (m *mockStore) AddComment(ctx context.Context, nodeID, text string) error {
	args := m.Called(ctx, nodeID, text)
	return args.Error(0)
}

func (m *mockStore) CreateProject(ctx context.Context, name, description string) (string, error) {
	args := m.Called(ctx, name, description)
	return args.String(0), args.Error(1)
}

func (m *mockStore) CreateNode(ctx context.Context, n store.NewNode) (string, error) {
	args := m.Called(ctx, n)
	return args.String(0), args.Error(1)
}
