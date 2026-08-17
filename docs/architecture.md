# Architecture

flowcontrol is one engine with many surfaces. The design goal: **one data model, one schema,
one set of generated bindings**, reachable natively, from Node, and from the browser.

## flowcore — the engine

`flowcore` (Rust) holds all the logic: the node tree, dependency evaluation, the status
cascade. It talks to storage through a narrow **`Sql` seam** (execute / query / transaction)
rather than a concrete database, so the same engine compiles two ways:

- **native** — over `rusqlite`, for a native daemon;
- **wasm32** — pure Rust, no C toolchain, calling back into a **host** for SQL
  (`__flowHostExec` / `__flowHostQuery`).

That host-import design is what lets the identical engine run inside a Node daemon _and_
directly in a browser tab.

## One proto, one dispatch

A single protobuf definition generates the bindings for every language (TypeScript, Go, Rust).
The engine exposes one synchronous `dispatch()` entry point, so a "mutation" is the same shape
whether it arrives from the web board, the CLI, or an agent.

## The daemon

`flowd` (and its Node twin, `flowd.js`) is the **single writer** to one SQLite file. It serves
the task-graph API over **gRPC-web/Connect on HTTP/1.1**, bound to `127.0.0.1`. The web board
is served from the same origin, which removes CORS, mixed-content, and certificate concerns in
one move. A **bearer token** (in `~/.flowcontrol/session.json`, mode 0600) guards the RPC
paths; the daemon also enforces a `Host` allow-list against DNS-rebinding.

## Lifecycle — the store, not the daemon

The daemon is disposable; **SQLite is the source of truth**. Every client runs the same
_ensure-on-connect_: connect to the daemon if one is up (discovered via `session.json`, guarded
by a single-instance lock), else spawn one. If the daemon dies, no data is lost — the next
client brings it back. Persistence is emergent: whichever client's process isn't torn down
keeps the daemon alive; when it goes, the data stays on disk.

## The browser

The standalone web build runs `flowcore` as wasm in a **Web Worker**, persisting to **OPFS**
via `@sqlite.org/sqlite-wasm` (a durable SAHPool VFS). It is the real engine over real SQLite —
fully functional, private to the browser, no network — which is what powers the GitHub Pages
demo.

## Transport, everywhere the same

Browser (`connect-web`), agent server (`connect-node`), and CLI (`connect-go`) all speak the
same gRPC-web/Connect dialect to the same loopback daemon. One transport choice keeps the
clients thin and makes a future hosted, multi-tenant deployment (the `Sql` seam over a cloud
database, keyed by a `workspace`) a natural extension rather than a rewrite.
