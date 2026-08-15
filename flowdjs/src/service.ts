// The FlowService implementation for flowd.js. Every unary RPC routes generically
// through the wasm `dispatch` (proto bytes in/out); Watch replays missed events
// then streams live mutations from the bus. This is the exact surface `flowd`
// (Rust) serves — flowcli, flowmcp, and flowui are transport-identical against both.
import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { Code, ConnectError, type HandlerContext, type ServiceImpl } from "@connectrpc/connect";
import {
  FlowService,
  PollChangesRequestSchema,
  PollChangesResponseSchema,
  WatchResponseSchema,
  type Mutation,
  type WatchResponse,
} from "@flow/api/flow/v1/flow_pb";

import type { Bus } from "./bus";
import type { Daemon } from "./store";

const HEARTBEAT_MS = 30_000;
// A replay window larger than this is a gap the client recovers from via
// GetSnapshot (matching the native daemon's retention proxy).
const REPLAY_LIMIT = 1000;

const CODES: Record<string, Code> = {
  not_found: Code.NotFound,
  invalid_argument: Code.InvalidArgument,
  failed_precondition: Code.FailedPrecondition,
  internal: Code.Internal,
};

/** Recover a ConnectError from the wasm's `"<code>: <message>"` JsError. */
function toConnectError(e: unknown): ConnectError {
  if (e instanceof ConnectError) return e;
  const msg = e instanceof Error ? e.message : String(e);
  const i = msg.indexOf(": ");
  const code = i > 0 ? (CODES[msg.slice(0, i)] ?? Code.Internal) : Code.Internal;
  const text = i > 0 && msg.slice(0, i) in CODES ? msg.slice(i + 2) : msg;
  return new ConnectError(text, code);
}

/**
 * Build the FlowService impl. Reflection over the service descriptor keeps this in
 * lockstep with the proto: every unary method is dispatched by its proto name, so
 * new RPCs need no code here.
 */
export function buildImpl(daemon: Daemon, bus: Bus): ServiceImpl<typeof FlowService> {
  const impl: Record<string, unknown> = {};
  for (const m of Object.values(FlowService.method)) {
    if (m.methodKind === "unary") {
      impl[m.localName] = (req: unknown) => {
        const bytes = toBinary(m.input, req as never);
        let out: Uint8Array;
        try {
          out = daemon.dispatch(m.name, bytes);
        } catch (e) {
          throw toConnectError(e);
        }
        const resp = fromBinary(m.output, out) as unknown as { mutation?: Mutation };
        // A write's response carries a Mutation; publish it for Watch subscribers.
        const mut = resp.mutation;
        if (mut && mut.events.length > 0) {
          bus.publish(mut.events[0]!.projectId, mut);
        }
        return resp;
      };
    } else if (m.name === "Watch") {
      impl[m.localName] = (req: unknown, ctx: HandlerContext) =>
        watchStream(daemon, bus, req as { projectId: string; fromSeq: bigint }, ctx);
    }
  }
  return impl as ServiceImpl<typeof FlowService>;
}

/** Server-streaming Watch: replay-then-live, with a 30s heartbeat — the exact
 *  contract flowd implements (WatchResponse.resync_required / .heartbeat). */
async function* watchStream(
  daemon: Daemon,
  bus: Bus,
  req: { projectId: string; fromSeq: bigint },
  ctx: HandlerContext,
): AsyncGenerator<WatchResponse> {
  const projectId = req.projectId;

  // Replay what the client missed since its last seq (PollChanges is the after-seq
  // read; the native Watch uses events_after with the same semantics).
  if (req.fromSeq > 0n) {
    const pcReq = toBinary(
      PollChangesRequestSchema,
      create(PollChangesRequestSchema, {
        projectId,
        afterSeq: req.fromSeq,
        limit: REPLAY_LIMIT + 1,
      }),
    );
    const pc = fromBinary(PollChangesResponseSchema, daemon.dispatch("PollChanges", pcReq));
    if (pc.events.length > 0) {
      const resync = pc.events.length > REPLAY_LIMIT;
      const events = resync ? pc.events.slice(0, REPLAY_LIMIT) : pc.events;
      yield create(WatchResponseSchema, {
        events,
        seq: events[events.length - 1]?.seq ?? req.fromSeq,
        resyncRequired: resync,
      });
    }
  }

  // Live: a queue fed by the bus + heartbeat, drained until the client disconnects.
  const queue: (Mutation | "HEARTBEAT")[] = [];
  let wake: (() => void) | null = null;
  const kick = () => {
    if (wake) {
      wake();
      wake = null;
    }
  };
  const onMut: (pid: string, mut: Mutation) => void = (pid, mut) => {
    if (pid === projectId) {
      queue.push(mut);
      kick();
    }
  };
  bus.add(onMut);
  const hb = setInterval(() => {
    queue.push("HEARTBEAT");
    kick();
  }, HEARTBEAT_MS);
  const onAbort = () => kick();
  ctx.signal.addEventListener("abort", onAbort);

  try {
    while (!ctx.signal.aborted) {
      if (queue.length === 0) {
        await new Promise<void>((resolve) => {
          wake = resolve;
        });
        continue;
      }
      const item = queue.shift()!;
      if (item === "HEARTBEAT") {
        yield create(WatchResponseSchema, { heartbeat: true });
      } else {
        yield create(WatchResponseSchema, {
          events: item.events,
          changedNodes: item.changedNodes,
          changedProgress: item.changedProgress,
          seq: item.seq,
        });
      }
    }
  } finally {
    clearInterval(hb);
    bus.remove(onMut);
    ctx.signal.removeEventListener("abort", onAbort);
  }
}
