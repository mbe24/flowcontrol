package ui

import (
	"strings"
	"testing"
)

// TestLanesRuleFullWidth guards that the horizontal rule under each lane
// header spans the full lane width — so the colored line reaches the lane's
// right border, not just the left. Previously the run length was capped by the
// status word (e.g. "READY" vs "DEFERRED"), so different lanes drew colored
// rules of visibly different lengths even though the row itself was padded.
func TestLanesRuleFullWidth(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenLanes
	m.width, m.height = 120, 30

	lines := strings.Split(m.View(), "\n")
	if len(lines) < 3 {
		t.Fatalf("view too short:\n%s", m.View())
	}
	// Rules sit on the third line: top border, lane headers, then the rules.
	rules := stripANSI(lines[2])

	var runs []int
	for _, seg := range strings.Fields(rules) {
		if strings.Contains(seg, "─") {
			runs = append(runs, wlen(seg))
		}
	}
	if len(runs) == 0 {
		t.Fatalf("no rule runs found; rule row %q", rules)
	}
	if len(runs) != 4 {
		t.Fatalf("expected 4 lane rule runs, got %d: %v", len(runs), runs)
	}

	// Lane widths differ by at most 1 (the remainder is spread over the last
	// lanes), so the runs must too. Any bigger spread means a rule was capped
	// by its status word instead of filling the lane.
	minR, maxR := runs[0], runs[0]
	for _, r := range runs {
		if r < minR {
			minR = r
		}
		if r > maxR {
			maxR = r
		}
	}
	if maxR-minR > 1 {
		t.Errorf("lane rule lengths spread %d (%v); rules should each span their full lane (differ by at most 1)",
			maxR-minR, runs)
	}
	// Each run must span essentially the whole lane column — far longer than
	// any status-word-capped rule (max status word len+4 is ~12), proving it
	// now reaches the right border.
	if minR < 16 {
		t.Errorf("lane rule runs too short (min %d, all %v); expected full-lane-width rules", minR, runs)
	}
}
