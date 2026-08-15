# flowd.js

The FlowControl daemon as a Node process — a drop-in for the Rust `flowd`. Same
proto, same port, same wire protocols, so `flowcli`, `flowmcp`, and `flowui` talk
to either without changes.

## How it works

```
flowcli / flowmcp (gRPC) ─┐
flowui (grpc-web) ────────┼─▶ connect-node (http2, allowHTTP1)
                          │      └─ FlowService impl (generic dispatch + Watch)
                          │            └─ flowcore.wasm  ── Sql seam ──▶ node:sqlite (durable WAL file)
```

- **`flowcore` compiled to wasm** holds all the logic (the store, the event log,
  FTS, undo) and is pure Rust — no SQLite inside it. It talks to a real database
  through two host functions (`__flowHostExec` / `__flowHostQuery`).
- **`node:sqlite`** is that real database: a durable file with WAL. This is where
  persistence actually lives.
- **connect-node** serves gRPC (h2c) and gRPC-web on one port; `Watch` is fanned
  out in-process from each committed mutation.

The schema is single-sourced from `flowcore` (`schema_sql()`), so both daemons
apply the exact same migrations.

## Requirements

- **Node ≥ 22.5** (uses the built-in `node:sqlite`). Tested on 22.19 and 24.
- The wasm artifact at `../flowwasm/pkg` (gitignored). Build it once:

  ```sh
  sh scripts/build-wasm.sh      # cargo + wasm-bindgen, via the flowd container
  ```

  Override the location with `FLOWWASM_PKG=/path/to/flowwasm.js` if needed.

## Run

```sh
npm run dev -w flowdjs -- --db ./flowd.sqlite --seed        # or:
node --experimental-sqlite --import tsx src/index.ts --db ./flowd.sqlite --seed
```

Flags / env: `--addr` (`FLOWD_ADDR`, default `127.0.0.1:50051`), `--db`
(`FLOWD_DB`, default `./flowd.sqlite`, use `:memory:` for ephemeral), `--seed`.

## Test

```sh
npm run test -w flowdjs         # spins the daemon up and drives it over gRPC
```

## Known limitations (v1)

- Dispatch is synchronous on the event loop (SQLite ops are fast and local; the
  Rust daemon offloads to a blocking pool — a future refinement here).
- `Watch` replay uses `PollChanges` (bounded at 1000 events → `resync_required`),
  matching the native daemon's retention behaviour.
