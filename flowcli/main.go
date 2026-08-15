// Command flowcli is the FlowControl terminal UI.
//
// It talks to a store.Store. By default it connects to a running daemon (flowd or
// flowd.js) over gRPC-web at 127.0.0.1:50051; `--demo` keeps the in-memory fixture
// so the UI can be explored without a server. Nothing under internal/ui changes
// either way.
//
// flowcli is a connect-only client: it does not start a daemon. In normal use one
// is already running — your agent's MCP ensures it, or you ran `flow ui`. If none
// is reachable, flowcli prints how to start one rather than erroring inside the UI
// (see plan/design.daemon-lifecycle.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
	"flowcli/internal/ui"
)

func main() {
	// FLOWCLI_ADDR sets the default server address (independent of flowd's own
	// bind), so the same binary works inside a container (via compose) and
	// natively; a literal -addr still wins.
	addrDefault := "127.0.0.1:50051"
	if v := os.Getenv("FLOWCLI_ADDR"); v != "" {
		addrDefault = v
	}
	var (
		demo   = flag.Bool("demo", false, "use the in-memory fixture instead of a daemon")
		addr   = flag.String("addr", addrDefault, "daemon address (gRPC-web over HTTP/1.1)")
		author = flag.String("author", "you", "author name attached to writes")
	)
	flag.Parse()

	var s store.Store
	if *demo {
		s = store.NewMemory()
	} else {
		g := store.NewGRPC(*addr, *author)
		if err := preflight(g); err != nil {
			fmt.Fprintf(os.Stderr,
				"flowcli: no FlowControl daemon reachable at %s.\n"+
					"  Start one with `flow ui` (it opens the app too), or it starts automatically\n"+
					"  when your agent (MCP) connects. Use --demo to explore offline.\n  (%v)\n",
				*addr, err)
			os.Exit(1)
		}
		s = g
	}

	p := tea.NewProgram(ui.New(s), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "flowcli:", err)
		os.Exit(1)
	}
}

// preflight makes one bounded call so a missing daemon becomes an actionable hint
// instead of an error surfaced inside the TUI. The daemon is stateless between
// calls, so this costs nothing beyond confirming reachability.
func preflight(s store.Store) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := s.Projects(ctx)
	return err
}
