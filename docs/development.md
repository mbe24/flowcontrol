# Development

## Repository layout

| Path | What |
| --- | --- |
| `flowcore/` | the Rust engine (native + wasm) over the `Sql` seam |
| `flowd/` | the native Rust daemon |
| `flowwasm/` | the wasm cdylib (flowcore for the browser / Node) |
| `flowdjs/` | the Node daemon — flowcore-as-wasm over `node:sqlite` |
| `flowmcp/` | the MCP server (TypeScript) |
| `flowui/` | the web board (Svelte) |
| `flowcli/` | the terminal UI (Go) |
| `shared/flowapi/` | generated protobuf bindings (internal) |
| `proto/` | the single proto + `buf` generation config |

`flowcore`, `flowd`, and `flowwasm` form a Cargo **workspace** (target/ at the repo root). The
TypeScript packages are a **pnpm** workspace.

## Prerequisites

- **Node ≥ 22.5** and **pnpm** via `corepack`
- **Rust** with the `wasm32-unknown-unknown` target (or **Docker**) to generate the wasm
- **Go** (for `flowcli`)

## Common commands

```sh
corepack pnpm install                 # install the TS workspace
corepack pnpm gen:wasm                # (re)generate the flowcore wasm glue

corepack pnpm --filter flowdjs dev    # run the Node daemon (from source)
corepack pnpm --filter flowdjs ui     # daemon + open the web board
corepack pnpm --filter flowui dev     # the in-browser demo (no daemon)

cd flowcli && go build -o flowcli . && ./flowcli   # the terminal UI
```

## Tests

```sh
corepack pnpm test:flowd       # flowcore / native daemon (Rust)
corepack pnpm --filter flowdjs test
corepack pnpm --filter flowmcp test
corepack pnpm test:flowcli     # Go
```

## CI

GitHub Actions builds and tests each language, including an **MCP integration** job that runs
the real `flowd` binary end-to-end. A separate, manually-triggered workflow deploys the
in-browser demo to GitHub Pages.

!!! note
    The wasm glue is generated, not committed. Run `corepack pnpm gen:wasm` after a clean
    checkout (CI does the equivalent natively).
