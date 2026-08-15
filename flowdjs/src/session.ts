// The liveness contract from design.daemon-lifecycle.md, made real.
//
// `session.json` (mode 0600, in ~/.flowcontrol) is the discovery file AND the
// single-instance marker: it records the daemon's addr, a bearer token, pid, and
// who spawned it. `ensureDaemon()` is the idempotent "connect, else spawn" every
// native entry point runs; the daemon itself uses `isAnotherDaemonLive()` to
// refuse to double-bind. Persistence is emergent — a daemon spawned by an
// un-sandboxed human client survives; one spawned by a sandboxed MCP dies with it,
// and the next client re-ensures. Nothing here fights the sandbox.
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import {
  chmodSync,
  mkdirSync,
  openSync,
  closeSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-node";
import { FlowService } from "@flow/api/flow/v1/flow_pb";

export type SpawnedBy = "mcp" | "cli" | "user";

export interface Session {
  /** gRPC/grpc-web base URL, e.g. "http://127.0.0.1:50051". */
  addr: string;
  /** Bearer token for the TCP listener (browser + remote). */
  token: string;
  pid: number;
  startedAt: number;
  spawnedBy: SpawnedBy;
}

// Paths are computed per call so FLOWD_HOME can be set at runtime (and per test).
function home(): string {
  return process.env.FLOWD_HOME ?? join(homedir(), ".flowcontrol");
}
function sessionFile(): string {
  return join(home(), "session.json");
}
function lockFile(): string {
  return join(home(), "daemon.lock");
}

// ── session.json read/write (0600) ───────────────────────────────────────────

export function readSession(): Session | null {
  try {
    return JSON.parse(readFileSync(sessionFile(), "utf8")) as Session;
  } catch {
    return null;
  }
}

export function writeSession(s: Session): void {
  mkdirSync(home(), { recursive: true, mode: 0o700 });
  writeFileSync(sessionFile(), JSON.stringify(s, null, 2), { mode: 0o600 });
  try {
    chmodSync(sessionFile(), 0o600); // ensure, even if the file pre-existed
  } catch {
    /* best effort on platforms without POSIX modes */
  }
}

export function clearSession(): void {
  try {
    rmSync(sessionFile());
  } catch {
    /* already gone */
  }
}

/**
 * A stable port + token across daemon generations: reuse the previous session's
 * so an already-open flowui tab reconnects to a resurrected daemon without a
 * reload. Falls back to a fresh token and a caller-provided default port.
 */
export function stablePortToken(defaultPort: number): { port: number; token: string } {
  const prev = readSession();
  const port = prev ? portOf(prev.addr) ?? defaultPort : defaultPort;
  // FLOW_TOKEN lets a deployment (or a cross-boundary client like flowcli in WSL/a
  // container, which can't read this session.json) share a provisioned token;
  // otherwise reuse the prior session's, else mint a fresh one.
  const token = process.env.FLOW_TOKEN ?? prev?.token ?? randomBytes(32).toString("base64url");
  return { port, token };
}

function portOf(addr: string): number | null {
  const m = addr.match(/:(\d+)$/);
  return m ? Number(m[1]) : null;
}

// ── liveness probe ────────────────────────────────────────────────────────────

/**
 * Is a daemon actually answering at `addr`? A real RPC (ListProjects) with a short
 * deadline — never a pid check, which lies after PID reuse.
 */
export async function probe(addr: string, timeoutMs = 1500): Promise<boolean> {
  try {
    const client = createClient(
      FlowService,
      createGrpcWebTransport({ baseUrl: addr, httpVersion: "1.1" })
    );
    await client.listProjects({ includeArchived: false }, { timeoutMs });
    return true;
  } catch (e) {
    // A gRPC-level status still means something answered; only a transport failure
    // (connection refused → Unavailable, or no answer → DeadlineExceeded) is "no
    // daemon". ConnectError.code is a numeric Code enum, not a string.
    if (e instanceof ConnectError) {
      return e.code !== Code.Unavailable && e.code !== Code.DeadlineExceeded;
    }
    return false;
  }
}

/** For the daemon's own startup guard: is a healthy peer already recorded + live? */
export async function isAnotherDaemonLive(): Promise<Session | null> {
  const s = readSession();
  return s && (await probe(s.addr)) ? s : null;
}

// ── ensure (connect, else spawn) ──────────────────────────────────────────────

/** How to launch flowd.js. Overridable for tests / packaged bins via FLOWD_SPAWN_CMD.
 *  Detects layout from this module's own extension: built dist/*.js runs the compiled
 *  entry with plain node; dev src/*.ts runs it under tsx. */
function spawnCommand(): { cmd: string; args: string[] } {
  const override = process.env.FLOWD_SPAWN_CMD;
  if (override) {
    const parts = JSON.parse(override) as string[];
    return { cmd: parts[0]!, args: parts.slice(1) };
  }
  if (fileURLToPath(import.meta.url).endsWith(".ts")) {
    const entry = fileURLToPath(new URL("./index.ts", import.meta.url));
    return { cmd: process.execPath, args: ["--import", "tsx", entry] };
  }
  const entry = fileURLToPath(new URL("./index.js", import.meta.url));
  return { cmd: process.execPath, args: [entry] };
}

/** Best-effort exclusive lock so two clients don't both spawn. Returns a release fn. */
async function acquireLock(timeoutMs = 8000): Promise<() => void> {
  mkdirSync(home(), { recursive: true, mode: 0o700 });
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const fd = openSync(lockFile(), "wx"); // O_CREAT | O_EXCL
      return () => {
        closeSync(fd);
        try {
          rmSync(lockFile());
        } catch {
          /* ignore */
        }
      };
    } catch {
      if (Date.now() > deadline) {
        // Assume a stale lock; steal it rather than deadlock forever.
        try {
          rmSync(lockFile());
        } catch {
          /* ignore */
        }
      }
      await delay(100);
    }
  }
}

/**
 * The idempotent ensure. Returns a live session, spawning flowd.js (detached) only
 * if nothing answers. Safe to call from every entry point concurrently.
 */
export async function ensureDaemon(opts: { spawnedBy: SpawnedBy; readyTimeoutMs?: number }): Promise<Session> {
  const existing = readSession();
  if (existing && (await probe(existing.addr))) return existing;

  const release = await acquireLock();
  try {
    // Re-check under the lock — another client may have spawned while we waited.
    const again = readSession();
    if (again && (await probe(again.addr))) return again;

    const { cmd, args } = spawnCommand();
    const child = spawn(cmd, args, {
      detached: true,
      stdio: "ignore",
      env: { ...process.env, FLOWD_SPAWNED_BY: opts.spawnedBy }
    });
    child.unref();

    return await waitForDaemon(opts.readyTimeoutMs ?? 10_000);
  } finally {
    release();
  }
}

async function waitForDaemon(timeoutMs: number): Promise<Session> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const s = readSession();
    if (s && (await probe(s.addr))) return s;
    if (Date.now() > deadline) throw new Error("flowd.js did not become ready in time");
    await delay(150);
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
