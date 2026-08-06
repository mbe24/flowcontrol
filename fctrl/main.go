// Command fctrl is the FlowControl terminal UI.
//
// It talks to a store.Store. Today that is an in-memory fixture; swap the one
// line in main for a client that speaks to the Rust core over a named pipe or
// gRPC and nothing in internal/ui changes.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"fctrl/internal/store"
	"fctrl/internal/ui"
)

func main() {
	s := store.NewMemory()

	p := tea.NewProgram(ui.New(s), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fctrl:", err)
		os.Exit(1)
	}
}
