// Ambient types for the generated wasm-bindgen (--target web) glue, which is
// gitignored (built into ./wasm/ by scripts/copy-wasm.sh). When the real
// flowwasm.d.ts is present next to the glue, TS prefers it; this keeps typecheck
// green on a fresh checkout where the glue hasn't been built yet.
declare module '*/wasm/flowwasm.js' {
  export default function init(moduleOrPath?: unknown): Promise<unknown>;
  export class Store {
    constructor();
    dispatch(method: string, req: Uint8Array): Uint8Array;
    free(): void;
  }
  export function schema_sql(): string;
  export function seed_sql(): string;
}
