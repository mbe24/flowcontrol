package ui

import (
	"strings"
	"testing"
)

// TestChainConnectorAlignment guards that a tree root's branching connector
// ("─┬") and its first child's corner connector ("└" / "├") are vertically
// aligned — i.e. the child's horizontal run reaches the root's vertical tee so
// the two lines connect. Previously the first-child gutter was two spaces,
// putting the corner one column to the right of the root's tee.
func TestChainConnectorAlignment(t *testing.T) {
	m := loadModel(t)
	m.screen = ScreenChain
	m.width, m.height = 100, 40

	var rootTee, childCorner int
	for _, ln := range strings.Split(m.View(), "\n") {
		p := stripANSI(ln)
		if strings.Contains(p, "─┬●") {
			rootTee = wlen(p[:strings.Index(p, "┬")])
		} else if strings.Contains(p, "└─●") || strings.Contains(p, "├─●") {
			sub := p
			// take the first connector on the line
			childCorner = wlen(sub[:strings.Index(sub, "└")])
		}
		if rootTee != 0 && childCorner != 0 {
			break
		}
	}
	if rootTee == 0 {
		t.Fatalf("no root connector (─┬) found in chain view:\n%s", m.View())
	}
	if childCorner == 0 {
		t.Fatalf("no child connector (└ / ├) found in chain view:\n%s", m.View())
	}
	if rootTee != childCorner {
		t.Errorf("root tee at col %d but child corner at col %d; they should be vertically aligned",
			rootTee, childCorner)
	}
}
