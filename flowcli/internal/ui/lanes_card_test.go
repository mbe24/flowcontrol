package ui

import (
	"strings"
	"testing"

	"flowcli/internal/store"
)

// TestLaneCardNoStrayBlank verifies that a lane card with a one-line title
// does not leave a stray blank row between the title and the work-package
// name. A short title should render as: header / title / work-package —
// directly stacked, with no empty line between title and work-package.
func TestLaneCardNoStrayBlank(t *testing.T) {
	m := loadModel(t)

	// find a task whose title fits on one line in a 22-wide lane so wrapTo
	// pads it with a second empty line.
	var found *store.Node
	var parent *store.Node
	for i := range m.nodes {
		n := m.nodes[i]
		if n.Type == store.Task && wlen(n.Title) <= 20 {
			found = &m.nodes[i]
			if p, ok := m.byID[n.ParentID]; ok {
				parent = &p
			}
			break
		}
	}
	if found == nil {
		t.Fatal("no short-title task in fixture")
	}

	// laneW=22 mirrors the former single-lane card width.
	cards := m.laneCard(*found, 22, false)

	// Walk the rendered card rows and assert the work-package (footer) line
	// directly follows the last non-empty title line — i.e. no blank between
	// the title block and the footer.
	var plains []string
	for _, c := range cards {
		plains = append(plains, stripANSI(c.plain))
	}
	// find the footer: the line containing the parent title text
	expected := ""
	if parent != nil {
		expected = parent.Title
	}
	footerIdx := -1
	for i, p := range plains {
		if strings.Contains(p, expected) && expected != "" {
			footerIdx = i
			break
		}
	}
	if footerIdx < 0 {
		t.Fatalf("could not locate work-package footer in card rows: %v", plains)
	}
	if footerIdx == 0 {
		t.Fatalf("footer unexpectedly the first row: %v", plains)
	}
	// the row immediately before the footer must be non-empty (the title)
	prev := strings.TrimSpace(plains[footerIdx-1])
	if prev == "" {
		t.Fatalf("blank row before work-package footer; card rows = %v", plains)
	}
}
