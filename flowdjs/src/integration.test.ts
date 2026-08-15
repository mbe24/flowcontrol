// End-to-end: start flowd.js on an ephemeral port and drive it with a real gRPC
// client (the same transport flowmcp/flowcli use). Exercises reads, a write (which
// goes through INSERT…RETURNING on node:sqlite), and live Watch fan-out.
import { rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-node";
import {
  CreateNodeRequestSchema,
  CreateNodeResponseSchema,
  FlowService,
  GetSnapshotRequestSchema,
  GetSnapshotResponseSchema,
  NodeKind,
  type WatchResponse,
} from "@flow/api/flow/v1/flow_pb";
import { afterAll, beforeAll, expect, test } from "vitest";

import { Bus } from "./bus";
import { startServer } from "./server";
import { createDaemon } from "./store";

let server: import("node:http").Server;
let client: ReturnType<typeof createClient<typeof FlowService>>;
const daemon = createDaemon({ dbPath: ":memory:", seed: true });

beforeAll(async () => {
  const started = await startServer({ daemon, bus: new Bus(), host: "127.0.0.1", port: 0 });
  server = started.server;
  client = createClient(
    FlowService,
    createGrpcWebTransport({ baseUrl: `http://127.0.0.1:${started.port}`, httpVersion: "1.1" }),
  );
});

afterAll(() => {
  server.close();
  daemon.close();
});

test("reads the seeded project over gRPC", async () => {
  const res = await client.listProjects({ includeArchived: false });
  expect(res.projects.map((p) => p.id)).toEqual(["prj-travel"]);
});

test("snapshot returns the seeded graph", async () => {
  const snap = await client.getSnapshot({ projectId: "prj-travel" });
  expect(snap.nodes.length).toBe(3);
  expect(snap.nodes.some((n) => n.id === "T-1042")).toBe(true);
});

test("FTS5 search works through node:sqlite", async () => {
  const res = await client.search({ projectId: "prj-travel", query: "device", limit: 10 });
  expect(res.nodes.some((n) => n.id === "T-1042")).toBe(true);
});

test("a write commits (INSERT…RETURNING) and is visible", async () => {
  const m = await client.createNode({
    projectId: "prj-travel",
    parentId: "WP-AUTH",
    kind: NodeKind.TASK,
    title: "created via flowd.js",
    meta: { author: "itest", idempotencyKey: "" },
  });
  const created = m.mutation?.changedNodes[0];
  expect(created?.id.startsWith("node-")).toBe(true);

  const snap = await client.getSnapshot({ projectId: "prj-travel" });
  expect(snap.nodes.some((n) => n.id === created?.id)).toBe(true);
});

test("Watch streams a live mutation", async () => {
  const ac = new AbortController();
  const received: WatchResponse[] = [];
  const done = (async () => {
    for await (const msg of client.watch({ projectId: "prj-travel", fromSeq: 0n }, { signal: ac.signal })) {
      if (!msg.heartbeat) {
        received.push(msg);
        break;
      }
    }
  })();

  // Let the stream establish, then make a change.
  await new Promise((r) => setTimeout(r, 150));
  await client.createNode({
    projectId: "prj-travel",
    parentId: "WP-AUTH",
    kind: NodeKind.TASK,
    title: "watch me",
    meta: { author: "itest", idempotencyKey: "" },
  });

  await done;
  ac.abort();
  expect(received[0]?.events.length ?? 0).toBeGreaterThan(0);
});

test("a durable file DB survives a daemon restart", () => {
  const file = join(tmpdir(), `flowdjs-test-${process.pid}.sqlite`);
  const cleanup = () => {
    for (const f of [file, `${file}-wal`, `${file}-shm`]) {
      try {
        rmSync(f);
      } catch {
        /* not there */
      }
    }
  };
  cleanup();

  // Run 1: seed + create a node, then close the daemon.
  const d1 = createDaemon({ dbPath: file, seed: true });
  const cn = fromBinary(
    CreateNodeResponseSchema,
    d1.dispatch(
      "CreateNode",
      toBinary(
        CreateNodeRequestSchema,
        create(CreateNodeRequestSchema, {
          projectId: "prj-travel",
          parentId: "WP-AUTH",
          kind: NodeKind.TASK,
          title: "persist me",
          meta: { author: "t", idempotencyKey: "" },
        }),
      ),
    ),
  );
  const id = cn.mutation?.changedNodes[0]?.id ?? "";
  expect(id).not.toBe("");
  d1.close();

  // Run 2: reopen the SAME file (no seed) — the node persists.
  const d2 = createDaemon({ dbPath: file, seed: false });
  const snap = fromBinary(
    GetSnapshotResponseSchema,
    d2.dispatch(
      "GetSnapshot",
      toBinary(GetSnapshotRequestSchema, create(GetSnapshotRequestSchema, { projectId: "prj-travel" })),
    ),
  );
  d2.close();
  cleanup();

  expect(snap.nodes.some((n) => n.id === id)).toBe(true);
});
