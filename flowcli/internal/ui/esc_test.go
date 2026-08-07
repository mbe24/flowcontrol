package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// press sends a key to the model and returns the updated model.
func press(t *testing.T, m Model, k string) Model {
	t.Helper()
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return upd.(Model)
}

// loadLanes returns a model on the lane screen with a card under the cursor,
// so current() finds a task and enter opens the detail view.
func loadLanes(t *testing.T) Model {
	t.Helper()
	m := loadModel(t)
	m.screen = ScreenLanes
	// find a non-empty lane
	lanes := m.laneSet()
	for li, st := range lanes {
		if len(m.laneTasks(st)) > 0 {
			m.lane = li
			break
		}
	}
	m.laneCursor[m.lane] = 0
	// populate selectedID like updateLanes does
	if n, ok := m.current(); ok {
		m.selectedID = n.ID
	}
	return m
}

// loadChain returns a model on the chain screen with a row under the cursor.
func loadChain(t *testing.T) Model {
	t.Helper()
	m := loadModel(t)
	m.screen = ScreenChain
	m.chainCursor = 0
	m.selectedID = ""
	if n, ok := m.current(); ok {
		m.selectedID = n.ID
	}
	return m
}

// TestEscReturnsToOriginatingView verifies that opening a task's detail from
// the lane or chain view and pressing ESC returns to that same view, not the
// tree.
func TestEscReturnsToOriginatingView(t *testing.T) {
	t.Run("from lanes", func(t *testing.T) {
		m := loadLanes(t)
		if m.screen != ScreenLanes {
			t.Fatalf("setup: expected lanes, got %v", m.screen)
		}

		m = press(t, m, "enter")
		if m.screen != ScreenDetail {
			t.Fatalf("after enter: expected detail, got %v", m.screen)
		}

		m = press(t, m, "esc")
		if m.screen != ScreenLanes {
			t.Errorf("after esc: expected to return to lanes, got %v", m.screen)
		}
	})

	t.Run("from chain", func(t *testing.T) {
		m := loadChain(t)
		if m.screen != ScreenChain {
			t.Fatalf("setup: expected chain, got %v", m.screen)
		}

		m = press(t, m, "enter")
		if m.screen != ScreenDetail {
			t.Fatalf("after enter: expected detail, got %v", m.screen)
		}

		m = press(t, m, "esc")
		if m.screen != ScreenChain {
			t.Errorf("after esc: expected to return to chain, got %v", m.screen)
		}
	})

	t.Run("from tree still returns to tree", func(t *testing.T) {
		m := loadModel(t)
		m.screen = ScreenTree
		if len(m.rows) == 0 {
			t.Skip("no tree rows in fixture")
		}
		m.cursor = 0
		m.selectedID = m.ownerTask(m.rows[0].node).ID

		m = press(t, m, "enter")
		if m.screen != ScreenDetail {
			t.Fatalf("after enter: expected detail, got %v", m.screen)
		}

		m = press(t, m, "esc")
		if m.screen != ScreenTree {
			t.Errorf("after esc: expected to return to tree, got %v", m.screen)
		}
	})
}
