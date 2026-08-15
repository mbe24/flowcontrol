// flowd.js entry point: the FlowControl daemon as a Node process. flowcore (wasm)
// over a durable node:sqlite file, serving FlowService (gRPC + grpc-web) — a
// drop-in for the Rust `flowd`, same proto, same port, so flowcli/flowmcp/flowui
// talk to either. Run: `tsx src/index.ts --db ./flowd.sqlite --seed`.
import { parseArgs } from "node:util";

import { Bus } from "./bus";
import { startServer } from "./server";
import { createDaemon } from "./store";

const { values } = parseArgs({
  options: {
    addr: { type: "string" },
    db: { type: "string" },
    seed: { type: "boolean" },
  },
});

const addr = values.addr ?? process.env.FLOWD_ADDR ?? "127.0.0.1:50051";
const dbPath = values.db ?? process.env.FLOWD_DB ?? "./flowd.sqlite";
const seed = values.seed ?? false;
const [host, portStr] = splitHostPort(addr);

const daemon = createDaemon({ dbPath, seed });
const bus = new Bus();

const { server } = await startServer({ daemon, bus, host, port: Number(portStr) });
console.error(
  `[flowd.js] listening on ${host}:${portStr} (gRPC + grpc-web) — db ${dbPath}`,
);

const shutdown = () => {
  server.close(() => {
    daemon.close();
    process.exit(0);
  });
};
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

/** Split `host:port` (tolerating an `http(s)://` prefix) with sane defaults. */
function splitHostPort(a: string): [string, string] {
  const s = a.replace(/^https?:\/\//, "");
  const idx = s.lastIndexOf(":");
  if (idx === -1) return [s || "127.0.0.1", "50051"];
  return [s.slice(0, idx) || "127.0.0.1", s.slice(idx + 1)];
}
