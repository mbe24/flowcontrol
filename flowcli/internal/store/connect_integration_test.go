package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestIntegrationAgainstDaemon drives the REAL connect-go gRPC-web transport
// against a live, seeded daemon (flowd.js or the Rust flowd). It is gated on
// FLOWCLI_IT_ADDR (e.g. http://flowdjs:50051) so the default `go test` stays
// hermetic; the flowcli/docker-compose.yml `it` profile sets it and brings a
// flowd.js up first. This is what proves flowcli ↔ flowd.js over the wire, not
// just the client construction.
func TestIntegrationAgainstDaemon(t *testing.T) {
	addr := os.Getenv("FLOWCLI_IT_ADDR")
	if addr == "" {
		t.Skip("set FLOWCLI_IT_ADDR to run the live daemon integration test")
	}
	s := NewGRPC(addr, os.Getenv("FLOWCLI_TOKEN"), "flowcli-it")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Read: the seed's project must come back over gRPC-web.
	projects, err := s.Projects(ctx)
	if err != nil {
		t.Fatalf("Projects over gRPC-web failed: %v", err)
	}
	var travel *Project
	for i := range projects {
		if projects[i].ID == "prj-travel" {
			travel = &projects[i]
		}
	}
	if travel == nil {
		t.Fatalf("expected seeded project prj-travel, got %v", projects)
	}

	// Write + read-back: a mutation commits on the daemon and the refetch sees it,
	// exercising the full request→response→invalidate→refetch path over the wire.
	id, err := s.CreateNode(ctx, NewNode{
		ProjectID: "prj-travel",
		ParentID:  "WP-AUTH",
		Type:      Task,
		Title:     "flowcli-it round-trip",
	})
	if err != nil {
		t.Fatalf("CreateNode over gRPC-web failed: %v", err)
	}
	nodes, err := s.Nodes(ctx, "prj-travel")
	if err != nil {
		t.Fatalf("Nodes after write failed: %v", err)
	}
	found := false
	for _, n := range nodes {
		if n.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("created node %q not present after refetch", id)
	}
	t.Logf("OK: %d projects; created+read node %s over gRPC-web/HTTP1", len(projects), id)
}
