import { createClient, type Client } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-node";
import { FlowService } from "@flow/api/flow/v1/flow_pb";

/** The generated flowd client. Every tool translates through this — no other state. */
export type FlowClient = Client<typeof FlowService>;

export const DEFAULT_FLOWD_ADDR = "http://127.0.0.1:50051";

/**
 * One shared channel to a daemon, over gRPC-web on HTTP/1.1. Works against both
 * flowd.js (HTTP/1.1 only) and the Rust flowd (tonic-web on the same axum port),
 * so flowmcp is daemon-agnostic. Reused for every call; the server is stateless
 * between calls.
 */
export function createFlowClient(
  addr: string = process.env.FLOWD_ADDR ?? DEFAULT_FLOWD_ADDR,
): FlowClient {
  return createClient(FlowService, createGrpcWebTransport({ baseUrl: addr, httpVersion: "1.1" }));
}
