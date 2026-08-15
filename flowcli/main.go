// Command flowcli is the FlowControl terminal UI.
//
// It talks to a store.Store. By default it connects to a running daemon (flowd or
// flowd.js) over gRPC-web at 127.0.0.1:50051; `--demo` keeps the in-memory fixture
// so the UI can be explored without a server. Nothing under internal/ui changes
// either way.
//
// flowcli is a connect-only client: it does not start a daemon. In normal use one
// is already running — your agent's MCP ensures it, or you ran `flow ui`. It
// discovers the address + bearer token from the daemon's session.json when they
// share a home. Across a boundary (WSL, a container, a different user) that file
// isn't shared, so pass --session-dir at the daemon's home, or --addr/--token
// directly (see plan/design.daemon-lifecycle.md).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowcli/internal/store"
	"flowcli/internal/ui"
)

func main() {
	// FLOWCLI_ADDR sets the default server address (independent of the daemon's own
	// bind), so the same binary works inside a container (via compose) and
	// natively; a literal -addr still wins.
	addrDefault := "127.0.0.1:50051"
	if v := os.Getenv("FLOWCLI_ADDR"); v != "" {
		addrDefault = v
	}
	var (
		demo       = flag.Bool("demo", false, "use the in-memory fixture instead of a daemon")
		addr       = flag.String("addr", addrDefault, "daemon address (gRPC-web over HTTP/1.1)")
		author     = flag.String("author", "you", "author name attached to writes")
		token      = flag.String("token", "", "bearer token (default: from session.json or FLOWCLI_TOKEN)")
		sessionDir = flag.String("session-dir", "", "dir holding the daemon's session.json (default: $FLOWD_HOME or ~/.flowcontrol)")
	)
	flag.Parse()

	var s store.Store
	if *demo {
		s = store.NewMemory()
	} else {
		// Discover addr + token from the daemon's session.json when co-located; each
		// can be overridden. `--token`/`FLOWCLI_TOKEN` and `--session-dir`/`FLOWD_HOME`
		// cover the cross-boundary (WSL/container) case where the file isn't shared.
		sess, _ := readSession(resolveSessionDir(*sessionDir))
		resolvedAddr := *addr
		if resolvedAddr == addrDefault && sess.Addr != "" {
			resolvedAddr = sess.Addr // the daemon may be on a non-default port
		}
		resolvedToken := firstNonEmpty(*token, os.Getenv("FLOWCLI_TOKEN"), sess.Token)

		g := store.NewGRPC(resolvedAddr, resolvedToken, *author)
		if err := preflight(g); err != nil {
			fmt.Fprintf(os.Stderr,
				"flowcli: no FlowControl daemon reachable at %s.\n"+
					"  Start one with `flow ui` (it opens the app too), or it starts automatically\n"+
					"  when your agent (MCP) connects. Across WSL/a container, point --session-dir at\n"+
					"  the daemon's home or pass --addr/--token. Use --demo to explore offline.\n  (%v)\n",
				resolvedAddr, err)
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

// session mirrors the fields flowcli needs from the daemon's session.json.
type session struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

// resolveSessionDir: --session-dir, else $FLOWD_HOME, else ~/.flowcontrol (matching
// the daemon's own default).
func resolveSessionDir(flagDir string) string {
	if flagDir != "" {
		return flagDir
	}
	if v := os.Getenv("FLOWD_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".flowcontrol"
	}
	return filepath.Join(home, ".flowcontrol")
}

func readSession(dir string) (session, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return session{}, false
	}
	var s session
	if json.Unmarshal(b, &s) != nil {
		return session{}, false
	}
	return s, true
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
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
