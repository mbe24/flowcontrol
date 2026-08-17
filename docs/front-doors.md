# Front doors

One data model, reached four ways. The first three are the doors you use; the fourth is the
process behind them.

## `flowui` — the web board

The browser view of the graph. It is a Svelte SPA **served by the daemon** same-origin, so
there is no CORS, no certificate, and no separate server to run — opening the board is what
starts (or connects to) the daemon. The daemon injects its bearer token into the page, so the
board authenticates automatically.

The same UI also builds as a **standalone demo** that runs the engine entirely in the browser
over OPFS — that is what is deployed to GitHub Pages, with no daemon and no agent.

## `flowcli` — the terminal UI

A Go TUI for people who live in the terminal. It connects to the same machine-local daemon
(reading `~/.flowcontrol/session.json` for the address and token) and renders the same graph.

## `flowmcp` — the agent server

An MCP (Model Context Protocol) server your coding agent speaks to over stdio. It exposes the
task graph as [tools](mcp-tools.md) — create projects and nodes, set status and dependencies,
search, undo. It ensures the daemon on startup and connects to it.

## `flowd` — the daemon

The process the three doors share. It is the **single writer** to one SQLite file and serves
the task-graph API (plus the web board) over gRPC-web/Connect on `127.0.0.1`. You rarely run
it by hand — every client starts it on demand. See [Architecture](architecture.md) for how the
daemon's lifecycle works and why the store, not the daemon, is the source of truth.

## How they share

All three clients speak the same transport (gRPC-web/Connect over HTTP/1.1) to the same
loopback daemon, guarded by a bearer token. Because the store is a plain SQLite file, the
daemon can come and go without losing anything.
