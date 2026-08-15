// Package store — the real client transport for the GRPC store: connect-go
// speaking gRPC-web over HTTP/1.1. That is the one protocol both daemons serve —
// flowd.js (HTTP/1.1 only) and the Rust flowd (tonic-web on the same port) — so
// the TUI is daemon-agnostic. (We dropped standard gRPC/h2c precisely because
// flowd.js doesn't serve it; see plan/design.daemon-lifecycle.md.)
package store

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	flowv1 "flowcli/internal/pb/flow/v1"
	"flowcli/internal/pb/flow/v1/flowv1connect"
)

// NewGRPC builds a Store that talks to a daemon over gRPC-web/HTTP-1.1. `baseURL`
// may be a bare host:port (normalised to http://); `token` (may be empty) is the
// bearer credential the daemon requires; `who` is the author byline on writes.
func NewGRPC(baseURL, token, who string) *GRPC {
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	opts := []connect.ClientOption{connect.WithGRPCWeb()}
	if token != "" {
		opts = append(opts, connect.WithInterceptors(bearer(token)))
	}
	client := flowv1connect.NewFlowServiceClient(http.DefaultClient, baseURL, opts...)
	return NewGRPCWithClient(&connectAdapter{c: client}, who)
}

// bearer attaches `Authorization: Bearer <token>` to every request.
func bearer(token string) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	})
}

// connectAdapter adapts the connect-go client to the local flowClient seam,
// unwrapping connect.Request/Response so the store code stays transport-agnostic.
type connectAdapter struct{ c flowv1connect.FlowServiceClient }

// unwrap collapses a (connect.Response[T], error) into (*T, error).
func unwrap[T any](res *connect.Response[T], err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *connectAdapter) ListProjects(ctx context.Context, in *flowv1.ListProjectsRequest) (*flowv1.ListProjectsResponse, error) {
	return unwrap(a.c.ListProjects(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) GetSnapshot(ctx context.Context, in *flowv1.GetSnapshotRequest) (*flowv1.GetSnapshotResponse, error) {
	return unwrap(a.c.GetSnapshot(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) SetStatus(ctx context.Context, in *flowv1.SetStatusRequest) (*flowv1.SetStatusResponse, error) {
	return unwrap(a.c.SetStatus(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) SetVerdict(ctx context.Context, in *flowv1.SetVerdictRequest) (*flowv1.SetVerdictResponse, error) {
	return unwrap(a.c.SetVerdict(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) AddComment(ctx context.Context, in *flowv1.AddCommentRequest) (*flowv1.AddCommentResponse, error) {
	return unwrap(a.c.AddComment(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) CreateProject(ctx context.Context, in *flowv1.CreateProjectRequest) (*flowv1.CreateProjectResponse, error) {
	return unwrap(a.c.CreateProject(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) CreateNode(ctx context.Context, in *flowv1.CreateNodeRequest) (*flowv1.CreateNodeResponse, error) {
	return unwrap(a.c.CreateNode(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) UpdateNode(ctx context.Context, in *flowv1.UpdateNodeRequest) (*flowv1.UpdateNodeResponse, error) {
	return unwrap(a.c.UpdateNode(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) DeleteNode(ctx context.Context, in *flowv1.DeleteNodeRequest) (*flowv1.DeleteNodeResponse, error) {
	return unwrap(a.c.DeleteNode(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) AddDependency(ctx context.Context, in *flowv1.AddDependencyRequest) (*flowv1.AddDependencyResponse, error) {
	return unwrap(a.c.AddDependency(ctx, connect.NewRequest(in)))
}
func (a *connectAdapter) RemoveDependency(ctx context.Context, in *flowv1.RemoveDependencyRequest) (*flowv1.RemoveDependencyResponse, error) {
	return unwrap(a.c.RemoveDependency(ctx, connect.NewRequest(in)))
}
