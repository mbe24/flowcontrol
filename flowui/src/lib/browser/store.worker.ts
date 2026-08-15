/// <reference lib="webworker" />
// The browser store worker: flowcore (pure-Rust wasm) driven over the Sql seam by
// @sqlite.org/sqlite-wasm on an OPFS SAHPool VFS — synchronous, durable, no
// COOP/COEP. The browser twin of flowd.js's node:sqlite host: same wasm, same two
// host functions, different backing SQLite. Handles one `dispatch` per message.
import sqlite3InitModule from '@sqlite.org/sqlite-wasm';

import initFlowwasm, { Store, schema_sql, seed_sql } from './wasm/flowwasm.js';

interface DispatchMsg {
  type: 'dispatch';
  id: number;
  method: string;
  req: Uint8Array;
}

let store: Store | undefined;

async function boot(): Promise<void> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const sqlite3: any = await sqlite3InitModule();
  const pool = await sqlite3.installOpfsSAHPoolVfs({ name: 'flow-opfs', clearOnInit: false });
  const db = new pool.OpfsSAHPoolDb('/flow.sqlite');
  db.exec('PRAGMA foreign_keys=ON;');

  await initFlowwasm();

  const bigToNum = (_k: string, v: unknown) => (typeof v === 'bigint' ? Number(v) : v);
  const host = self as unknown as {
    __flowHostExec: (sql: string, params: string) => string;
    __flowHostQuery: (sql: string, params: string) => string;
  };

  host.__flowHostExec = (sql, paramsJson) => {
    try {
      const params = JSON.parse(paramsJson) as unknown[];
      db.exec({ sql, bind: params.length ? params : undefined });
      return JSON.stringify({
        changes: db.changes(),
        lastInsertRowid: Number(sqlite3.capi.sqlite3_last_insert_rowid(db.pointer))
      });
    } catch (e) {
      return JSON.stringify({ error: msg(e) });
    }
  };
  host.__flowHostQuery = (sql, paramsJson) => {
    try {
      const params = JSON.parse(paramsJson) as unknown[];
      const rows: unknown[] = [];
      db.exec({ sql, bind: params.length ? params : undefined, rowMode: 'object', resultRows: rows });
      return JSON.stringify({ rows }, bigToNum);
    } catch (e) {
      return JSON.stringify({ error: msg(e) });
    }
  };

  const ver = Number(db.selectValue('PRAGMA user_version'));
  if (ver < 1) {
    db.exec(schema_sql());
    db.exec(seed_sql());
    db.exec('PRAGMA user_version=1;');
  }
  store = new Store();
}

// Boot eagerly; each dispatch awaits readiness.
const ready = boot();

self.onmessage = async (ev: MessageEvent<DispatchMsg>) => {
  const m = ev.data;
  if (m?.type !== 'dispatch') return;
  try {
    await ready;
    const bytes = store!.dispatch(m.method, m.req);
    self.postMessage({ id: m.id, bytes });
  } catch (e) {
    self.postMessage({ id: m.id, error: msg(e) });
  }
};

function msg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
