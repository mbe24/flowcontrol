# flowcontrol

**flowcontrol** is a **dependency-aware task graph** for humans and the agents working
alongside them. Work packages hold tasks, tasks hold verifiable steps, and dependencies
cross any level — mark one node **DONE** and the engine re-evaluates everything downstream.
One data model, three front doors: a **web app**, a **terminal UI**, and an **MCP server**.

!!! note "Pre-release"
    The core is built and running. The npm packages, the published command surface, and
    these docs are being finalized — expect names and commands to still move.

## Why

- **One graph, shared live** — an agent adds tasks over MCP; you watch them appear on the
  board or in the terminal. Same store, same moment.
- **The engine derives readiness** — you never set _READY_ or _BLOCKED_; they fall out of the
  dependency edges. Flip a blocker to _DONE_ and its dependents unblock automatically.
- **Three front doors, one model** — the web board, the terminal UI, and the agent server all
  read and write the same task graph.
- **Local & private by default** — a single daemon owns one SQLite file on your machine and
  serves everything over loopback with a bearer token. No cloud, no account.
- **The daemon is not the store** — SQLite is. Any client starts the daemon on demand; if it
  dies, your data is untouched and the next client brings it back.

## At a glance

The hierarchy is fixed — **work package → task → step** — and dependencies may cross levels:

```
WORK PACKAGE  Release-UX development
  TASK        Wire the board launcher            [DONE]
  TASK        Bundle the web build into the daemon [READY]  ← unblocked when the task above went DONE
    STEP      copy flowui/dist → flowd/dist/ui
    STEP      default uiDir to the built path
```

## Next

- [Concepts](concepts.md) — the data model and how the engine derives status.
- [Installation](installation.md) — run it from source today; npm is coming.
- [Front doors](front-doors.md) — the web board, terminal UI, and agent server.
- [MCP tools](mcp-tools.md) — the tools your agent calls.
- [Architecture](architecture.md) — one engine, native and in the browser.
