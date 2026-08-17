# flowcontrol

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/mbe24/flowcontrol/main/assets/logo/logo-mark-dark-512.png">
  <img src="https://raw.githubusercontent.com/mbe24/flowcontrol/main/assets/logo/logo-mark-light-512.png" alt="flowcontrol logo: one node unblocks two — the dependency cascade the engine performs when a node turns DONE" width="150" align="right">
</picture>

[![CI](https://github.com/mbe24/flowcontrol/actions/workflows/ci.yml/badge.svg)](https://github.com/mbe24/flowcontrol/actions/workflows/ci.yml)
![npm](https://img.shields.io/badge/npm-not_yet_published-lightgrey)
![Docs](https://img.shields.io/badge/docs-coming_soon-lightgrey)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-orange.svg)](LICENSE)

**flowcontrol** is a **dependency-aware task graph** for humans and the agents working
alongside them. Work packages hold tasks, tasks hold verifiable steps, and dependencies
cross any level — mark one node **DONE** and the engine re-evaluates everything downstream.
One data model, three front doors: a **web app**, a **terminal UI**, and an **MCP server**.

- **One graph, shared live** — an agent adds tasks over MCP; you watch them appear on the
  board or in the terminal. Same store, same moment.
- **The engine derives readiness** — you never set _READY_ or _BLOCKED_; they fall out of the
  dependency edges. Flip a blocker to _DONE_ and its dependents unblock automatically.
- **Three front doors, one model** — `flowui` (web), `flowcli` (terminal), `flowmcp` (agents).
  Pick any; they all read and write the same task graph.
- **Local & private by default** — a single daemon owns one SQLite file on your machine and
  serves everything over loopback with a bearer token. No cloud, no account.
- **The daemon is not the store** — SQLite is. Any client starts the daemon on demand; if it
  dies, your data is untouched and the next client brings it back.

> **Status: pre-release.** The core is built and running; the npm packages, the published
> command surface, and the docs are being finalized. See [the roadmap](#roadmap).

## The pieces

| Package | Role | Language | Ships as |
| --- | --- | --- | --- |
| `flowmcp` | MCP server — the agent's door into the graph | TypeScript | npm _(soon)_ |
| `flowd` | the daemon — single writer to SQLite, serves the API + web UI | TypeScript / Node | npm _(soon)_ |
| `flowui` | the web board | Svelte | served by `flowd` / GitHub Pages |
| `flowcli` | the terminal UI | Go | npm _(soon)_ + `go install` |
| `flowcore` | the engine — task graph over a portable `Sql` seam | Rust | internal (native + wasm) |

`flowcore` compiles to native (for a Rust daemon) and to WebAssembly, so the very same engine
runs inside the Node daemon and directly in the browser.

## How it works

Every node carries a **declared** status you set — `OPEN`, `DEFERRED`, or `DONE` — and an
**effective** status the engine computes from the dependency graph:

| | meaning |
| --- | --- |
| 🟢 **READY** | every blocker is `DONE` — workable now (derived) |
| 🔴 **BLOCKED** | still waiting on a blocker (derived) |
| ⚪ **DEFERRED** | paused on purpose |
| 🔵 **DONE** | complete — marking it re-evaluates everything downstream |

Dependencies may cross levels (a step can block a whole work package), and cycles are
rejected. That single rule — _mark one node DONE, the cascade re-flows_ — is the whole idea,
and it's what the logo shows: one filled node releasing two hollow ones.

## Quickstart (from source)

Pre-release, so today you run it from the repo. Requires **Node ≥ 22.5**, **pnpm** (via
`corepack`), and a Rust toolchain (or Docker) to generate the wasm.

```sh
corepack pnpm install
corepack pnpm gen:wasm               # build the flowcore wasm
corepack pnpm --filter flowui build  # build the web board (daemon-connected)
corepack pnpm --filter flowdjs ui    # start the daemon + open the board in your browser
```

Point your agent at the from-source MCP server (any MCP client — Claude Code, Claude
Desktop, Cursor, …):

```json
{
  "mcpServers": {
    "flowcontrol": {
      "command": "node",
      "args": ["--import", "tsx", "<path>/flowcontrol/flowmcp/src/index.ts"]
    }
  }
}
```

The MCP connects to the same machine-local daemon the board uses, so tasks your agent
creates show up on the board immediately.

### Coming soon (npm)

Once published, the whole install is one command per front door — no toolchain required:

```sh
npm i -g @mbe24/flowmcp @mbe24/flowcli   # the agent server + the terminal UI
flowui                                    # open the web board (starts the daemon)
```

## Roadmap

- **npm release** of `@mbe24/flowmcp`, `@mbe24/flowd`, `@mbe24/flowcli` (+ a one-command
  `flowmcp install` to write your MCP host config).
- **Live board updates** over a streaming subscription (no reload, no polling).
- **`go install` + prebuilt binaries** for `flowcli`.
- **Hosted, multi-tenant** flowcontrol on the same `Sql` seam.

## Documentation

Full documentation is on its way. For now, the [Quickstart](#quickstart-from-source) above and
the in-repo design notes are the reference.

## License

[Apache 2.0](LICENSE) © Mikael Beyene.
