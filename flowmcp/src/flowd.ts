import { createClient, type Client, type Interceptor } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-node";
import { FlowService } from "@flow/api/flow/v1/flow_pb";

/** The generated flowd client. Every tool translates through this — no other state. */
export type FlowClient = Client<typeof FlowService>;

export const DEFAULT_FLOWD_ADDR = "http://127.0.0.1:50051";

/** Attach `Authorization: Bearer <token>` to every request (the daemon requires it). */
function bearer(token: string): Interceptor {
  return (next) => (req) => {
    req.header.set("Authorization", `Bearer ${token}`);
    return next(req);
  };
}

/**
 * One shared channel to a daemon, over gRPC-web on HTTP/1.1. Works against both
 * flowd.js (HTTP/1.1 only) and the Rust flowd (tonic-web on the same axum port),
 * so flowmcp is daemon-agnostic. `token` (from ensureDaemon's session, or
 * FLOWD_TOKEN for an explicit addr) authenticates the calls; empty means no auth
 * (e.g. a daemon started without a token). Reused for every call; the server is
 * stateless between calls.
 */
export function createFlowClient(
  addr: string = process.env.FLOWD_ADDR ?? DEFAULT_FLOWD_ADDR,
  token = process.env.FLOWD_TOKEN ?? "",
): FlowClient {
  const interceptors = token ? [bearer(token)] : [];
  return createClient(
    FlowService,
    createGrpcWebTransport({ baseUrl: addr, httpVersion: "1.1", interceptors }),
  );
}
