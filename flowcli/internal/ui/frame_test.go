package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/mattn/go-runewidth"

	"flowcli/internal/store"
)

// TestFrameAlignment runs the real TUI program headlessly through teatest and
// verifies that every rendered frame line has the same display width. This is
// a regression test for the top border falling short of the right wall: the
// header was measured with len() (bytes) instead of display cells, so its "┐"
// corner never reached the right border. If the frame ever drifts again, the
// mismatched widths fail here.
func TestFrameAlignment(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		New(store.NewMemory()),
		teatest.WithInitialTermSize(100, 40),
	)
	t.Cleanup(func() {
		_ = tm.Quit()
	})

	// Let the initial load + first render land before we grab the model.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("flowcli"))
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(20*time.Millisecond))

	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}

	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	if fm == nil {
		t.Fatal("no final model")
	}

	// Assert the alignment invariant on the model's own rendered view: every
	// non-empty line (header, body rows, separator, keys, footer) shares one
	// display width, so the top-right corner meets the right wall.
	widths := lineWidths(fm.View())
	if len(widths) == 0 {
		t.Fatal("rendered view had no lines")
	}
	first := widths[0]
	for i, w := range widths {
		if w != first {
			t.Errorf("frame line %d width %d != first line width %d", i, w, first)
		}
	}
}

func lineWidths(view string) []int {
	var out []int
	for _, line := range strings.Split(view, "\n") {
		w := runewidth.StringWidth(stripANSI(line))
		if w == 0 {
			continue // skip fully-blank lines
		}
		out = append(out, w)
	}
	return out
}

var _ = tea.Model(nil) // keep bubbletea import for forward-compat with teatest API
