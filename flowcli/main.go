// Command flowcli is the FlowControl terminal UI.
//
// It talks to a store.Store. By default it opens gRPC to the flowd core
// (127.0.0.1:50051); `--demo` keeps the in-memory fixture so the UI can be
// explored without a running server. Nothing under internal/ui changes either
// way.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
		demo   = flag.Bool("demo", false, "use the in-memory fixture instead of the flowd server")
		addr   = flag.String("addr", addrDefault, "flowd gRPC address")
		author = flag.String("author", "you", "author name attached to writes")
	)
	flag.Parse()

	var s store.Store
	if *demo {
		s = store.NewMemory()
	} else {
		conn, err := dial(*addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "flowcli: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()
		s = store.NewGRPC(conn, *author)
	}

	p := tea.NewProgram(ui.New(s), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "flowcli:", err)
		os.Exit(1)
	}
}

// dial opens a gRPC connection to the flowd core. grpc.NewClient dials lazily,
// so the first store call triggers the connection — letting the CLI start
// alongside the server without a hard requirement that it be up first.
func dial(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
