package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// UpdateNode title + condition: edits apply, and editing the condition marks
// the agent report stale (per the designer's "! editing marks stale" rule).
func TestMemoryUpdateNode(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	t2 := findNode(m, "T-1042")
	require.NotNil(t, t2)
	require.NotEqual(t, "", t2.Condition)

	title := "Rewritten title"
	cond := "npm run test:new"
	require.NoError(t, m.UpdateNode(ctx, "T-1042", NodeUpdate{Title: &title, Condition: &cond}))

	got := findNode(m, "T-1042")
	require.NotNil(t, got)
	require.Equal(t, "Rewritten title", got.Title)
	require.Equal(t, "npm run test:new", got.Condition)
	require.Equal(t, NoReport, got.Verification.Agent, "editing condition must mark report stale")
}

// Deleting a node with a descendant removes both, plus any edge touching them.
func TestMemoryDeleteNodeCascades(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// T-1041 has steps and is referenced in the fixture deps.
	require.NoError(t, m.DeleteNode(ctx, "T-1046"))

	require.Nil(t, findNode(m, "T-1046"))
	// verify at least one descendant was removed (T-1046.1 is a step)
	require.Nil(t, findNode(m, "T-1046.1"))
}

// Delete of an unknown node errors.
func TestMemoryDeleteNodeNotFound(t *testing.T) {
	m := NewMemory()
	err := m.DeleteNode(context.Background(), "DOES-NOT-EXIST")
	require.Error(t, err)
}

// AddDependency dedupes; RemoveDependency drops the edge.
func TestMemoryAddRemoveDependency(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	require.NoError(t, m.AddDependency(ctx, "T-1041", "T-7001"))
	// second add of same edge is a no-op
	require.NoError(t, m.AddDependency(ctx, "T-1041", "T-7001"))
	deps := listDeps(m)
	count := 0
	for _, d := range deps {
		if d.BlockerID == "T-1041" && d.BlockedID == "T-7001" {
			count++
		}
	}
	require.Equal(t, 1, count, "duplicate edge must be deduped")

	require.NoError(t, m.RemoveDependency(ctx, "T-1041", "T-7001"))
	deps = listDeps(m)
	count = 0
	for _, d := range deps {
		if d.BlockerID == "T-1041" && d.BlockedID == "T-7001" {
			count++
		}
	}
	require.Equal(t, 0, count, "edge must be removed")
}

func findNode(m *Memory, id string) *Node {
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			return &m.nodes[i]
		}
	}
	return nil
}

func listDeps(m *Memory) []Dependency {
	return m.deps
}
