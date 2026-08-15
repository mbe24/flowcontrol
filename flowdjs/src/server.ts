// The socket. connect-node serves Connect + gRPC + gRPC-web from one handler; an
// http2 server with allowHTTP1 accepts both h2c gRPC (flowcli/flowmcp) and
// HTTP/1.1 gRPC-web (flowui). CORS mirrors flowd's CorsLayer so the browser can call.
import * as http2 from "node:http2";

import { cors } from "@connectrpc/connect";
import { connectNodeAdapter } from "@connectrpc/connect-node";
import { FlowService } from "@flow/api/flow/v1/flow_pb";

import type { Bus } from "./bus";
import { buildImpl } from "./service";
import type { Daemon } from "./store";

export function startServer(opts: {
  daemon: Daemon;
  bus: Bus;
  host: string;
  port: number;
}): Promise<{ server: http2.Http2Server; port: number }> {
  const adapter = connectNodeAdapter({
    routes: (router) => {
      router.service(FlowService, buildImpl(opts.daemon, opts.bus));
    },
  });
  // allowHTTP1 lets one h2 server accept both h2c gRPC and HTTP/1.1 grpc-web. It is
  // valid at runtime but missing from @types/node's plain-server options.
  const server = http2.createServer(
    { allowHTTP1: true } as unknown as http2.ServerOptions,
    withCors(adapter as unknown as NodeHandler),
  );
  return new Promise((resolve) => {
    server.listen(opts.port, opts.host, () => {
      const address = server.address();
      const port = typeof address === "object" && address ? address.port : opts.port;
      resolve({ server, port });
    });
  });
}

type NodeHandler = (
  req: http2.Http2ServerRequest,
  res: http2.Http2ServerResponse,
) => void;

/** Add permissive CORS + preflight so browser grpc-web can reach the daemon. */
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
