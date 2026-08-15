import { randomUUID } from "node:crypto";

/**
 * Build the `WriteMeta` init for a mutation. flowmcp mints the idempotency key
 * (never the model), so the wrapper's own transport retries dedupe via flowd's
 * replay; `author` is a display label only, never an identity/authorization input.
 * Used by the mutation tools (M3c).
 */
export function writeMeta(
  author: string,
  idempotencyKey: string = randomUUID(),
): { author: string; idempotencyKey: string } {
  return { author, idempotencyKey };
}
