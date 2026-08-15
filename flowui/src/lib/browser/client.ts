// A FlowService client whose unary RPCs run against the in-browser wasm store
// (flowcore over OPFS in a Web Worker) instead of the network. RemoteStore takes
// this in place of its grpc-web client, so the demo/offline build runs the REAL
// engine — durable, correct-by-construction — with no UI changes.
import { fromBinary, toBinary } from '@bufbuild/protobuf';
import { Code, ConnectError, type Client } from '@connectrpc/connect';
import { FlowService } from '@flow/api/flow/v1/flow_pb';

const CODES: Record<string, Code> = {
  not_found: Code.NotFound,
  invalid_argument: Code.InvalidArgument,
  failed_precondition: Code.FailedPrecondition,
  internal: Code.Internal
};

/** Create a FlowService client backed by the wasm store worker. */
export function createBrowserClient(): Client<typeof FlowService> {
  const worker = new Worker(new URL('./store.worker.ts', import.meta.url), { type: 'module' });
  let seq = 0;
  const pending = new Map<number, { resolve: (b: Uint8Array) => void; reject: (e: unknown) => void }>();

  worker.onmessage = (ev: MessageEvent<{ id: number; bytes?: Uint8Array; error?: string }>) => {
    const { id, bytes, error } = ev.data;
    const p = pending.get(id);
    if (!p) return;
    pending.delete(id);
    if (error !== undefined) p.reject(toConnectError(error));
    else p.resolve(bytes!);
  };

  const dispatch = (method: string, req: Uint8Array): Promise<Uint8Array> =>
    new Promise((resolve, reject) => {
      const id = ++seq;
      pending.set(id, { resolve, reject });
      worker.postMessage({ type: 'dispatch', id, method, req });
    });

  const client: Record<string, unknown> = {};
  for (const m of Object.values(FlowService.method)) {
    if (m.methodKind !== 'unary') continue;
    client[m.localName] = async (reqMsg: unknown) => {
      const bytes = await dispatch(m.name, toBinary(m.input, reqMsg as never));
      return fromBinary(m.output, bytes);
    };
  }
  return client as unknown as Client<typeof FlowService>;
}

/** Recover a ConnectError from the wasm's `"<code>: <message>"` JsError string. */
function toConnectError(msg: string): ConnectError {
  const i = msg.indexOf(': ');
  const known = i > 0 && msg.slice(0, i) in CODES;
  const code = known ? CODES[msg.slice(0, i)] : Code.Internal;
  return new ConnectError(known ? msg.slice(i + 2) : msg, code);
}
