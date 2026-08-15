// flowd.js entry point: the FlowControl daemon as a Node process. flowcore (wasm)
// over a durable node:sqlite file, serving FlowService (gRPC + grpc-web) — a
// drop-in for the Rust `flowd`, same proto, same port. Run:
// `tsx src/index.ts --db ./flowd.sqlite --seed`.
//
// Lifecycle (see plan/design.daemon-lifecycle.md): single-instance via session.json,
// stable port+token across restarts so open flowui tabs reconnect. Started either
// by a human (`flow ui`/flowcli — persists) or best-effort by flowmcp.
import { parseArgs } from "node:util";

import { Bus } from "./bus";
import { startServer } from "./server";
import {
  clearSession,
  isAnotherDaemonLive,
  stablePortToken,
  writeSession,
  type SpawnedBy
} from "./session";
import { createDaemon } from "./store";

const { values } = parseArgs({
  options: {
    addr: { type: "string" },
    db: { type: "string" },
    seed: { type: "boolean" }
  }
});

const DEFAULT_PORT = 50051;
const explicit = values.addr ?? process.env.FLOWD_ADDR;
const dbPath = values.db ?? process.env.FLOWD_DB ?? "./flowd.sqlite";
const seed = values.seed ?? false;
const spawnedBy: SpawnedBy = (process.env.FLOWD_SPAWNED_BY as SpawnedBy) ?? "user";

// Refuse to double-bind: if a healthy daemon is already recorded, defer to it.
const live = await isAnotherDaemonLive();
if (live) {
  console.error(`[flowd.js] already running at ${live.addr} (pid ${live.pid}) — nothing to do`);
  process.exit(0);
}

// Stable port + token across generations (reused from the last session if present).
const { port: stablePort, token } = stablePortToken(DEFAULT_PORT);
const [host, port] = explicit ? splitHostPort(explicit) : ["127.0.0.1", stablePort];

const daemon = createDaemon({ dbPath, seed });
const bus = new Bus();

const { server } = await startServer({ daemon, bus, host, port });
const addr = `http://${host}:${port}`;
writeSession({ addr, token, pid: process.pid, startedAt: Date.now(), spawnedBy });
console.error(
  `[flowd.js] listening on ${host}:${port} (gRPC + grpc-web) — db ${dbPath} — spawned-by ${spawnedBy}`
);

const shutdown = () => {
  clearSession();
  server.close(() => {
    daemon.close();
    process.exit(0);
  });
};
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

/** Split `host:port` (tolerating an `http(s)://` prefix) with sane defaults. */
function splitHostPort(a: string): [string, number] {
  const s = a.replace(/^https?:\/\//, "");
  const idx = s.lastIndexOf(":");
  if (idx === -1) return [s || "127.0.0.1", DEFAULT_PORT];
  return [s.slice(0, idx) || "127.0.0.1", Number(s.slice(idx + 1)) || DEFAULT_PORT];
}
