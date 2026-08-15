import { createClient, type Client } from "@connectrpc/connect";
import { createGrpcTransport } from "@connectrpc/connect-node";
import { FlowService } from "@flow/api/flow/v1/flow_pb";

/** The generated flowd client. Every tool translates through this — no other state. */
export type FlowClient = Client<typeof FlowService>;

export const DEFAULT_FLOWD_ADDR = "http://127.0.0.1:50051";

/**
 * One shared gRPC channel to flowd over native HTTP/2 (h2c on http://). Reused for
 * every call; the server holds no state of its own between calls.
 */
export function createFlowClient(
  addr: string = process.env.FLOWD_ADDR ?? DEFAULT_FLOWD_ADDR,
): FlowClient {
  return createClient(FlowService, createGrpcTransport({ baseUrl: addr }));
}
