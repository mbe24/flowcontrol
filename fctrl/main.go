package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"flowcontrol/fctrl/internal/store"
	"flowcontrol/fctrl/internal/ui"
)

func main() {
	// Swap this line for a pipe or gRPC client once the Rust core is listening:
	// st, err := pipestore.Dial("/tmp/fctrl.sock")
	st := store.NewMemory()

	p := tea.NewProgram(ui.New(st), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fctrl:", err)
		os.Exit(1)
	}
}
