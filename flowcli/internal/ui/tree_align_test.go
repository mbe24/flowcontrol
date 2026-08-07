package ui

import (
	"strings"
	"testing"
)

// contentRightEdge returns the display-width column (relative to the content,
// with leading "│ " frame stripped) of the last non-space character before the
// closing frame wall. It ignores the trailing padding and the "│" wall that
// frame() adds to every row, so it measures where the row's actual content
// ends rather than the box.
func contentRightEdge(line string) int {
	// strip the walls of the frame box
	s := strings.TrimPrefix(line, "│")
	s = strings.TrimSuffix(s, "│")
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return 0
	}
	// account for the leading space after the left wall
	return wlen(s)
}

// TestTreeRightAlignment verifies that in the tree view the work-package
// state+ratio tail and the task condition+ratio tail are both flush to the
// same content right edge, i.e. they right-align within the frame rather than
// ending at columns that vary with title length.
func TestTreeRightAlignment(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.width, m.height = 100, 40

	lines := strings.Split(m.View(), "\n")
	var wpEdge, taskEdge int
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "│") || !strings.HasSuffix(ln, "│") {
			continue
		}
		plain := stripANSI(ln)
		if strings.Contains(plain, "%") && strings.Contains(plain, "█") {
			// work-package row: percentage + progress bar
			if e := contentRightEdge(plain); e > wpEdge {
				wpEdge = e
			}
		} else if strings.Contains(plain, "T-") && strings.Contains(plain, "/") {
			// task row: has an ID and a step ratio
			if e := contentRightEdge(plain); e > taskEdge {
				taskEdge = e
			}
		}
	}
	if wpEdge == 0 {
		t.Fatalf("no work-package rows found; view:\n%s", m.View())
	}
	if taskEdge == 0 {
		t.Fatalf("no task rows found; view:\n%s", m.View())
	}
	if wpEdge != taskEdge {
		t.Errorf("WP state/ratio tail (%-3d) and task condition/ratio tail (%-3d) do not share the same right edge",
			wpEdge, taskEdge)
	}
}
