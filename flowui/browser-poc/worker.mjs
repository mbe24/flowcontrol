// Browser store worker: @sqlite.org/sqlite-wasm on an OPFS SAHPool (durable, no
// COOP/COEP) backs the two host functions the pure-Rust flowwasm imports. Proves
// flowcore's SQL runs in the browser with real persistence — the browser twin of
// flowd.js's node:sqlite host.
import sqlite3InitModule from "./sqlite3/index.mjs";
import initFlowwasm, { Store, schema_sql, seed_sql } from "./flowwasm/flowwasm.js";

const log = (message) => postMessage({ type: "log", message });

let store;

async function boot() {
  const sqlite3 = await sqlite3InitModule({ print: () => {}, printErr: (m) => log("sqlite: " + m) });
  log("sqlite3 " + sqlite3.version.libVersion);
  if (!sqlite3.installOpfsSAHPoolVfs) throw new Error("no OPFS SAHPool VFS in this build");

  const pool = await sqlite3.installOpfsSAHPoolVfs({ name: "flow-opfs", clearOnInit: false });
  const db = new pool.OpfsSAHPoolDb("/flow.sqlite");
  db.exec("PRAGMA foreign_keys=ON;");

  await initFlowwasm();
  log("flowwasm initialised");

  const bigToNum = (_k, v) => (typeof v === "bigint" ? Number(v) : v);

  globalThis.__flowHostExec = (sql, paramsJson) => {
    try {
      const params = JSON.parse(paramsJson);
      db.exec({ sql, bind: params.length ? params : undefined });
      return JSON.stringify({
        changes: db.changes(),
        lastInsertRowid: Number(sqlite3.capi.sqlite3_last_insert_rowid(db.pointer)),
      });
    } catch (e) {
      return JSON.stringify({ error: String((e && e.message) || e) });
    }
  };
  globalThis.__flowHostQuery = (sql, paramsJson) => {
    try {
      const params = JSON.parse(paramsJson);
      const rows = [];
      db.exec({ sql, bind: params.length ? params : undefined, rowMode: "object", resultRows: rows });
      return JSON.stringify({ rows }, bigToNum);
    } catch (e) {
      return JSON.stringify({ error: String((e && e.message) || e) });
    }
  };

  const ver = Number(db.selectValue("PRAGMA user_version"));
  if (ver < 1) {
    db.exec(schema_sql());
    db.exec(seed_sql());
    db.exec("PRAGMA user_version=1;");
    log("migrated + seeded (fresh OPFS db)");
  } else {
    log("existing OPFS db (user_version=" + ver + ")");
  }

  store = new Store();
  log("store ready");
  return { ver };
}

onmessage = async (ev) => {
  const msg = ev.data;
  if (msg.type === "boot") {
    try {
      const r = await boot();
      postMessage({ type: "booted", ...r });
    } catch (e) {
      postMessage({ type: "error", message: String((e && e.stack) || e) });
    }
  } else if (msg.type === "dispatch") {
    try {
      const out = store.dispatch(msg.method, msg.req);
      postMessage({ type: "result", id: msg.id, len: out.length, bytes: out });
    } catch (e) {
      postMessage({ type: "result", id: msg.id, error: String((e && e.message) || e) });
    }
  }
};
