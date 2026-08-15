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

/** Add permissive CORS + preflight so browser gRPC-web can reach the daemon. */
function withCors(handler: NodeHandler): NodeHandler {
  const allowMethods = cors.allowedMethods.join(", ");
  const allowHeaders = [...cors.allowedHeaders, "Authorization"].join(", ");
  const exposeHeaders = cors.exposedHeaders.join(", ");
  return (req, res) => {
    const origin = req.headers.origin;
    if (typeof origin === "string") {
      res.setHeader("Access-Control-Allow-Origin", origin);
      res.setHeader("Access-Control-Allow-Methods", allowMethods);
      res.setHeader("Access-Control-Allow-Headers", allowHeaders);
      res.setHeader("Access-Control-Expose-Headers", exposeHeaders);
      res.setHeader("Access-Control-Max-Age", "7200");
    }
    if (req.method === "OPTIONS") {
      res.writeHead(204);
      res.end();
      return;
    }
    handler(req, res);
  };
}
