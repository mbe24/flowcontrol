// The socket. One plain HTTP/1.1 server (http.createServer) serves everything the
// browser needs same-origin: Connect + gRPC-web for the API, static assets for the
// flowui SPA (via the adapter's `fallback`). HTTP/1.1 is deliberate — a browser
// speaks it for the static bundle, and gRPC-web works over it; Node's h2c
// `allowHTTP1` downgrade is unreliable, and native gRPC-over-h2 clients get a
// separate UDS listener later (design.transport.md). CORS mirrors flowd's CorsLayer.
import * as http from "node:http";

import { cors } from "@connectrpc/connect";
import { connectNodeAdapter } from "@connectrpc/connect-node";
import { FlowService } from "@flow/api/flow/v1/flow_pb";

import type { Bus } from "./bus";
import { buildImpl } from "./service";
import { staticHandler } from "./static";
import type { Daemon } from "./store";

type Fallback = NonNullable<Parameters<typeof connectNodeAdapter>[0]["fallback"]>;
type NodeHandler = http.RequestListener;

export function startServer(opts: {
  daemon: Daemon;
  bus: Bus;
  host: string;
  port: number;
  /** If set, serve the flowui SPA from this dir at every non-RPC path (same origin). */
  uiDir?: string;
}): Promise<{ server: http.Server; port: number }> {
  const adapter = connectNodeAdapter({
    routes: (router) => {
      router.service(FlowService, buildImpl(opts.daemon, opts.bus));
    },
    // Non-RPC paths → the SPA (if a bundle dir was given).
    fallback: opts.uiDir ? (staticHandler(opts.uiDir) as unknown as Fallback) : undefined
  });
  const server = http.createServer(withCors(adapter as unknown as NodeHandler));
  return new Promise((resolve) => {
    server.listen(opts.port, opts.host, () => {
      const address = server.address();
      const port = typeof address === "object" && address ? address.port : opts.port;
      resolve({ server, port });
    });
  });
}

// "localhost is not private": any page in your browser can hit 127.0.0.1. Two
// cheap, server-only defenses (see plan/design.daemon-lifecycle.md, transport doc):
//  1. Host-header allowlist — the request must be addressed to a loopback host, so
//     a rebound public name (DNS-rebinding) is rejected.
//  2. Origin allowlist — reflect Access-Control-Allow-Origin ONLY for loopback
//     origins (so a public site like evil.com gets no CORS grant and its preflight
//     fails) plus any explicitly configured FLOW_ALLOWED_ORIGINS. Non-browser
//     clients (flowmcp/flowcli) send no Origin and ignore CORS; they're gated by
//     the Host check. This deliberately still allows any loopback origin, so the
//     Vite dev server (localhost:5173 → daemon) keeps working with no config.
// The bearer token (a later step) is the additional layer against local processes.
const LOOPBACK = new Set(["127.0.0.1", "localhost", "::1"]);
const EXTRA_ORIGINS = new Set(
  (process.env.FLOW_ALLOWED_ORIGINS ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
);
// Extra Host values to accept beyond loopback — e.g. a compose service name
// ("flowdjs:50051"), a LAN host, or a hosted domain. Default empty, so the daemon
// is loopback-only out of the box; this is the knob that also makes cross-host
// integration tests possible without weakening the default (bind with --addr 0.0.0.0).
const EXTRA_HOSTS = new Set(
  (process.env.FLOW_ALLOWED_HOSTS ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
);

/** The hostname portion of a `Host`/authority value, minus port and IPv6 brackets. */
function hostOnly(host: string): string {
  if (host.startsWith("[")) return host.slice(1, host.indexOf("]")); // [::1]:port
  const i = host.lastIndexOf(":");
  return i === -1 ? host : host.slice(0, i);
}

function isAllowedHost(host: string | undefined): boolean {
  if (typeof host !== "string") return false;
  if (LOOPBACK.has(hostOnly(host))) return true;
  // Match either the full authority ("flowdjs:50051") or just the hostname.
  return EXTRA_HOSTS.has(host) || EXTRA_HOSTS.has(hostOnly(host));
}

function isAllowedOrigin(origin: string): boolean {
  if (EXTRA_ORIGINS.has(origin)) return true;
  try {
    return LOOPBACK.has(new URL(origin).hostname);
  } catch {
    return false;
  }
}

function withCors(handler: NodeHandler): NodeHandler {
  const allowMethods = cors.allowedMethods.join(", ");
  const allowHeaders = [...cors.allowedHeaders, "Authorization"].join(", ");
  const exposeHeaders = cors.exposedHeaders.join(", ");
  return (req, res) => {
    // DNS-rebind defense: only serve requests addressed to a loopback host (or an
    // explicitly allowed one via FLOW_ALLOWED_HOSTS).
    if (!isAllowedHost(req.headers.host)) {
      res.statusCode = 421; // Misdirected Request
      res.end("bad host");
      return;
    }
    const origin = req.headers.origin;
    if (typeof origin === "string" && isAllowedOrigin(origin)) {
      res.setHeader("Access-Control-Allow-Origin", origin);
      res.setHeader("Vary", "Origin");
      res.setHeader("Access-Control-Allow-Methods", allowMethods);
      res.setHeader("Access-Control-Allow-Headers", allowHeaders);
      res.setHeader("Access-Control-Expose-Headers", exposeHeaders);
      res.setHeader("Access-Control-Max-Age", "7200");
    }
    // A disallowed Origin simply gets no CORS grant → the browser blocks its
    // preflight/response. OPTIONS still returns 204 (with grant only if allowed).
    if (req.method === "OPTIONS") {
      res.writeHead(204);
      res.end();
      return;
    }
    handler(req, res);
  };
}
