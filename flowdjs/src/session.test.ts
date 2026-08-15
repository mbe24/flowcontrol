// Exercises the liveness contract: session.json roundtrip, stable port+token,
// the liveness probe, connect-if-present, and a real spawn-if-absent (which boots
// an actual flowd.js and reaps it).
import { mkdtempSync, rmSync, statSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, beforeEach, expect, test } from "vitest";

import { Bus } from "./bus";
import { startServer } from "./server";
import {
  ensureDaemon,
  isAnotherDaemonLive,
  probe,
  readSession,
  stablePortToken,
  writeSession
} from "./session";
import { createDaemon } from "./store";

let homeDir: string;

beforeEach(() => {
  homeDir = mkdtempSync(join(tmpdir(), "flowd-home-"));
  process.env.FLOWD_HOME = homeDir;
});

afterEach(() => {
  delete process.env.FLOWD_HOME;
  delete process.env.FLOWD_ADDR;
  delete process.env.FLOWD_DB;
  try {
    rmSync(homeDir, { recursive: true, force: true });
  } catch {
    /* ignore */
  }
});

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = createServer();
    s.listen(0, "127.0.0.1", () => {
      const port = (s.address() as { port: number }).port;
      s.close(() => resolve(port));
    });
    s.on("error", reject);
  });
}

test("session.json roundtrips (0600) and stablePortToken reuses it", () => {
  writeSession({ addr: "http://127.0.0.1:51234", token: "tok-abc", pid: 42, startedAt: 1, spawnedBy: "cli" });
  const s = readSession();
  expect(s?.addr).toBe("http://127.0.0.1:51234");
  expect(s?.token).toBe("tok-abc");
  // POSIX perms only assert where they exist.
  if (process.platform !== "win32") {
    expect(statSync(join(homeDir, "session.json")).mode & 0o777).toBe(0o600);
  }
  const { port, token } = stablePortToken(50051);
  expect(port).toBe(51234); // reused from the previous session
  expect(token).toBe("tok-abc");
});

test("probe is false when nothing is listening", async () => {
  const port = await freePort(); // free = nobody there
  expect(await probe(`http://127.0.0.1:${port}`, 800)).toBe(false);
});

test("connect-if-present: ensureDaemon returns the live daemon without spawning", async () => {
  const daemon = createDaemon({ dbPath: ":memory:", seed: true });
  const { server, port } = await startServer({ daemon, bus: new Bus(), host: "127.0.0.1", port: 0 });
  const addr = `http://127.0.0.1:${port}`;
  writeSession({ addr, token: "t", pid: process.pid, startedAt: Date.now(), spawnedBy: "user" });
  try {
    expect(await probe(addr)).toBe(true);
    expect((await isAnotherDaemonLive())?.addr).toBe(addr);
    const ensured = await ensureDaemon({ spawnedBy: "mcp" });
    expect(ensured.addr).toBe(addr); // reused, not a fresh spawn
    expect(ensured.pid).toBe(process.pid);
  } finally {
    server.close();
    daemon.close();
  }
});

test("spawn-if-absent: ensureDaemon boots a real flowd.js and it self-registers", async () => {
  const port = await freePort();
  process.env.FLOWD_ADDR = `127.0.0.1:${port}`;
  process.env.FLOWD_DB = ":memory:";
  expect(readSession()).toBeNull(); // nothing running yet

  const ensured = await ensureDaemon({ spawnedBy: "cli", readyTimeoutMs: 20_000 });
  try {
    expect(ensured.addr).toBe(`http://127.0.0.1:${port}`);
    expect(ensured.spawnedBy).toBe("cli"); // provenance propagated via env
    expect(await probe(ensured.addr)).toBe(true);
  } finally {
    try {
      process.kill(ensured.pid);
    } catch {
      /* already gone */
    }
  }
}, 30_000);
