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

// TestTreeBarConditionAlignment verifies that the work-package step-ratio bar
// starts at the same column as the task condition text, so the bar visually
// lines up under the conditions of the tasks below it.
func TestTreeBarConditionAlignment(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.width, m.height = 100, 40

	lines := strings.Split(m.View(), "\n")
	var barStart, condStart int

	for _, ln := range lines {
		if !strings.HasPrefix(ln, "│") || !strings.HasSuffix(ln, "│") {
			continue
		}
		plain := stripANSI(ln)
		if strings.Contains(plain, "█") && strings.Contains(plain, "%") {
			barStart = wlen(plain[:strings.Index(plain, "█")])
		} else if strings.Contains(plain, "T-") && strings.Contains(plain, "/") {
			// The ratio is the rightmost field (ratioW cells wide) and is
			// preceded by one space then the condition field (condW cells
			// wide), all right-aligned. Derive the condition's start column
			// from the row's right edge so the assertion is independent of the
			// (ambiguous) glyph markers — task IDs like "T-1043" and condition
			// text like "--grep" both contain dashes.
			edge := contentRightEdge(plain)
			ratioBegin := edge - ratioW + 1
			condStart = ratioBegin - 1 - condW
		}
	}
	if barStart == 0 {
		t.Fatalf("no WP bar found; view:\n%s", m.View())
	}
	if condStart == 0 {
		t.Fatalf("no task condition found; view:\n%s", m.View())
	}
	if barStart != condStart {
		t.Errorf("WP step-ratio bar starts at col %d but task condition starts at col %d; they should align",
			barStart, condStart)
	}
}

// TestTreeProjectBarAlignment verifies that the project summary row's
// step-ratio bar starts at the same column as the work-package bar (and the
// task condition column), and that the project percent right-aligns with the
// WP percent. This keeps the top summary line visually in step with the rows
// below it.
func TestTreeProjectBarAlignment(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenTree
	m.width, m.height = 100, 40

	lines := strings.Split(m.View(), "\n")
	var projBar, projPct, wpBar, wpPct int
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "│ ") || !strings.HasSuffix(ln, "│") {
			continue
		}
		plain := stripANSI(ln)
		// project summary: starts with the READY/BLOCKED/DEFER/DONE counts
		if strings.Contains(plain, "READY") && strings.Contains(plain, "%") && strings.Contains(plain, "█") {
			projBar = wlen(plain[:strings.Index(plain, "█")])
			projPct = wlen(plain[:strings.Index(plain, "%")])
		} else if strings.Contains(plain, "%") && strings.Contains(plain, "█") && strings.Contains(plain, "ACTIVE") {
			wpBar = wlen(plain[:strings.Index(plain, "█")])
			wpPct = wlen(plain[:strings.Index(plain, "%")])
		}
	}
	if projBar == 0 || wpBar == 0 {
		t.Fatalf("could not find project/WP bars; view:\n%s", m.View())
	}
	if projBar != wpBar {
		t.Errorf("project bar starts at col %d but WP bar starts at col %d; they should align",
			projBar, wpBar)
	}
	if projPct != wpPct {
		t.Errorf("project percent ends at col %d but WP percent ends at col %d; they should align",
			projPct, wpPct)
	}
}
