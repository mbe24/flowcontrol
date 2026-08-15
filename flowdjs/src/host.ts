// The host SQLite for flowd.js: a real `node:sqlite` database (durable WAL file or
// ":memory:") that the pure-Rust wasm drives over the `Sql` seam. We install the
// two synchronous bridge functions the wasm imports — `__flowHostExec` and
// `__flowHostQuery` — backed by this connection. SQLite lives HERE, not in the
// wasm; that is what makes persistence real.
import { createRequire } from "node:module";
import type { DatabaseSync, StatementSync } from "node:sqlite";

// Load node:sqlite via a runtime require, not a static import: bundlers (esbuild)
// strip the `node:` prefix from static builtin imports, and there is no bare
// `sqlite` builtin, so a bundled `import … from "node:sqlite"` fails at runtime.
// The type import above is type-only (erased), so it doesn't hit that path.
const sqlite = createRequire(import.meta.url)("node:sqlite") as typeof import("node:sqlite");

export interface Host {
  db: DatabaseSync;
  close(): void;
}

/**
 * Open the host SQLite, bring it to `schemaSql` (guarded by `PRAGMA user_version`,
 * mirroring the native daemon), optionally seed it, and wire the wasm's host
 * imports onto `globalThis`. The wasm looks these up lazily at dispatch time.
 */
export function openHost(dbPath: string, schemaSql: string, seedSql: string | null): Host {
  const db = new sqlite.DatabaseSync(dbPath);
  // FK enforcement is required for the schema's cascade deletes; WAL + busy timeout
  // smooth concurrent reads on a file DB (no-ops on ":memory:").
  db.exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;");

  const row = db.prepare("PRAGMA user_version").get() as { user_version?: number } | undefined;
  if (Number(row?.user_version ?? 0) < 1) {
    db.exec(schemaSql);
    if (seedSql) db.exec(seedSql);
    db.exec("PRAGMA user_version = 1;");
  }

  // The store issues many small, repeated statements per RPC — cache the compiled
  // form. A cached StatementSync is safe to re-run: run()/all() rebind each call.
  const cache = new Map<string, StatementSync>();
  const prep = (sql: string): StatementSync => {
    let s = cache.get(sql);
    if (!s) {
      s = db.prepare(sql);
      cache.set(sql, s);
    }
    return s;
  };
  // node:sqlite returns INTEGER as bigint when a value exceeds 2^53 — normalise to
  // number for JSON (our ids/timestamps/seqs are well within range).
  const bigToNum = (_k: string, v: unknown) => (typeof v === "bigint" ? Number(v) : v);

  // flowcore's SQL uses numbered `?N` placeholders. node:sqlite binds those by NAME
  // (the number), not by position — positional binding throws "column index out of
  // range" on Node 22.19 (22.23 is laxer, but named works on both). Map the param
  // array to a { "1": v1, "2": v2, … } object.
  type Bindable = string | number | bigint | null | Uint8Array;
  const named = (params: unknown[]): Record<string, Bindable> => {
    const o: Record<string, Bindable> = {};
    for (let i = 0; i < params.length; i++) o[String(i + 1)] = params[i] as Bindable;
    return o;
  };

  const g = globalThis as unknown as {
    __flowHostExec: (sql: string, params: string) => string;
    __flowHostQuery: (sql: string, params: string) => string;
  };

  g.__flowHostExec = (sql, paramsJson) => {
    try {
      const params = JSON.parse(paramsJson) as unknown[];
      const stmt = prep(sql);
      const info = params.length ? stmt.run(named(params)) : stmt.run();
      return JSON.stringify({
        changes: Number(info.changes),
        lastInsertRowid: Number(info.lastInsertRowid),
      });
    } catch (e) {
      return JSON.stringify({ error: errMsg(e) });
    }
  };
  g.__flowHostQuery = (sql, paramsJson) => {
    try {
      const params = JSON.parse(paramsJson) as unknown[];
      const stmt = prep(sql);
      const rows = params.length ? stmt.all(named(params)) : stmt.all();
      return JSON.stringify({ rows }, bigToNum);
    } catch (e) {
      return JSON.stringify({ error: errMsg(e) });
    }
  };

  return { db, close: () => db.close() };
}

function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
