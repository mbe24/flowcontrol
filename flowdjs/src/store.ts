// Loads the flowcore wasm and binds it to a host SQLite, exposing a single
// bytes-in/bytes-out `dispatch` — "the service minus the socket". The service
// layer wraps this with the FlowService routes and Watch fan-out.
import { existsSync } from "node:fs";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

import { openHost, type Host } from "./host";

const require = createRequire(import.meta.url);

export interface Daemon {
  /** Run one unary RPC by proto name (e.g. "CreateNode"): request bytes → response bytes. */
  dispatch(method: string, req: Uint8Array): Uint8Array;
  close(): void;
}

interface WasmModule {
  Store: new () => { dispatch(method: string, req: Uint8Array): Uint8Array };
  schema_sql(): string;
  seed_sql(): string;
}

/** The wasm-bindgen (nodejs) glue built from `flowwasm`. Override with FLOWWASM_PKG.
 *  Built package: bundled at dist/wasm/. Dev (tsx): the repo's flowwasm/pkg. */
function wasmPkgPath(): string {
  if (process.env.FLOWWASM_PKG) return process.env.FLOWWASM_PKG;
  for (const rel of ["./wasm/flowwasm.js", "../../flowwasm/pkg/flowwasm.js"]) {
    const p = fileURLToPath(new URL(rel, import.meta.url));
    if (existsSync(p)) return p;
  }
  return fileURLToPath(new URL("../../flowwasm/pkg/flowwasm.js", import.meta.url));
}

/**
 * Load the wasm, open the host SQLite (durable file or ":memory:"), migrate/seed,
 * and return the dispatcher. The wasm's schema is the single source of truth — we
 * apply `schema_sql()` to the host DB so both daemons share one schema.
 */
export function createDaemon(opts: { dbPath: string; seed: boolean }): Daemon {
  const wasm = require(wasmPkgPath()) as WasmModule;
  const host: Host = openHost(
    opts.dbPath,
    wasm.schema_sql(),
    opts.seed ? wasm.seed_sql() : null,
  );
  const store = new wasm.Store();
  return {
    dispatch: (method, req) => store.dispatch(method, req),
    close: () => host.close(),
  };
}
