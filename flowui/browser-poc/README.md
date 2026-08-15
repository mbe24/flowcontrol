# Browser store — proof of concept

Proves flowcore runs **fully in the browser** with **durable** persistence, using
the same pure-Rust `flowwasm` the native daemon and `flowd.js` use — only the host
functions differ.

```
main thread ──postMessage──▶ Web Worker
                              ├─ flowwasm (pure-Rust, --target web)  ── Sql seam ──┐
                              └─ @sqlite.org/sqlite-wasm on an OPFS SAHPool VFS ◀───┘
                                   (durable, no COOP/COEP required)
```

The worker installs `__flowHostExec` / `__flowHostQuery` — the same two host
functions `flowd.js` backs with `node:sqlite` — but here they run SQLite in wasm on
an **OPFS SAHPool** file, which persists across reloads and browser restarts. The
SAHPool VFS gives *synchronous* SQLite (what the seam needs) and needs no
cross-origin isolation headers.

## What it demonstrates (verified in Chrome)

- sqlite 3.53 + flowwasm boot in a Worker; schema applied from `schema_sql()`.
- `ListProjects`, `Search` (**FTS5**), `CreateNode` (**transaction + INSERT…RETURNING**),
  `GetSnapshot` — all through the wasm `dispatch`.
- **Durability**: reload the page → `user_version=1`, the DB is reused, and the
  snapshot grows because the previous write persisted in OPFS.

## Run

```sh
sh setup.sh                 # builds the --target web glue + copies sqlite-wasm assets
node server.mjs . 8099      # any static server with correct wasm MIME on localhost
# open http://127.0.0.1:8099/  (localhost is a secure context → OPFS works)
```

## Next (productionising into flowui)

- Move `worker.mjs` to `src/lib/browser/store.worker.ts` (TS) and load the glue as a
  Vite asset.
- Wrap it with connect-es `createRouterTransport` so flowui's existing
  `createClient(FlowService, …)` drives the worker in-process — no other UI change.
- Serve the built SPA from GitHub Pages / any static host (no server needed).
