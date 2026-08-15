import { create } from "@bufbuild/protobuf";
import type { CallToolResult } from "@modelcontextprotocol/server";
import { randomUUID } from "node:crypto";
import { z, type ZodType } from "zod";
import {
  AgentResult,
  DeclaredStatus,
  EffectiveStatus,
  EventKind,
  HumanVerdict,
  type Mutation,
  NodeKind,
  WorkPackageState,
  type WriteMeta,
  WriteMetaSchema,
} from "@flow/api/flow/v1/flow_pb";
import type { ToolDeps } from "./registry";

// ── enum name <-> number (tool schemas use proto SCREAMING names; wire uses ints)

export const nodeKindName = (v: NodeKind): string => NodeKind[v] ?? String(v);
export const effStatusName = (v: EffectiveStatus): string =>
  EffectiveStatus[v] ?? String(v);
export const declStatusName = (v: DeclaredStatus): string =>
  DeclaredStatus[v] ?? String(v);
export const wpStateName = (v: WorkPackageState): string =>
  WorkPackageState[v] ?? String(v);
export const eventKindName = (v: EventKind): string => EventKind[v] ?? String(v);
export const agentResultName = (v: AgentResult): string =>
  AgentResult[v] ?? String(v);
export const humanVerdictName = (v: HumanVerdict): string =>
  HumanVerdict[v] ?? String(v);

/** Numeric value for a proto-name string that a zod enum already validated. */
export function enumFromName(
  e: Record<string, string | number>,
  name: string,
): number {
  return e[name] as number;
}

// ── writes: WriteMeta + the shared Mutation projection

/** Unwrap a mutation-bearing response; flowd always sets it on success. */
export function requireMutation(m: Mutation | undefined): Mutation {
  if (!m) throw new Error("flowd returned no mutation");
  return m;
}

/** WriteMeta for a mutation: the wrapper mints the idempotency key (never the
 *  model); `author` is a display label only. Plan §5.1/§5.5. */
export function writeMeta(deps: ToolDeps): WriteMeta {
  return create(WriteMetaSchema, {
    author: deps.author,
    idempotencyKey: randomUUID(),
  });
}

/** The shared write-result structured shape: `{seq, changed:[{id,status}]}`. */
export function mutationStructured(
  m: Mutation,
  extra: Record<string, unknown> = {},
): Record<string, unknown> {
  const changed = m.changedNodes.map((n) => ({
    id: n.id,
    status: effStatusName(n.status),
  }));
  return { seq: Number(m.seq), changed, ...extra };
}

/** "Changed: id=STATUS, …" or "" when nothing cascaded. */
export function changedSummary(m: Mutation): string {
  if (m.changedNodes.length === 0) return "";
  return `Changed: ${m.changedNodes
    .map((n) => `${n.id}=${effStatusName(n.status)}`)
    .join(", ")}.`;
}

// ── outputSchema helpers (advertised in tools/list; validate structuredContent)

/** The `changed` cascade array shape, shared by every write's outputSchema. */
export const changedShape = z.array(
  z.object({ id: z.string(), status: z.string() }),
);

/** Output shape for a mutation-returning write: `{seq, changed, …extra}`. */
export function writeOutputSchema(
  extra: Record<string, ZodType> = {},
): Record<string, ZodType> {
  return { seq: z.number(), changed: changedShape, ...extra };
}

/** Compose a write tool result: one-line text (+ cascade) and structuredContent. */
export function writeResult(
  text: string,
  m: Mutation,
  extra?: Record<string, unknown>,
): CallToolResult {
  const tail = changedSummary(m);
  return {
    content: [{ type: "text", text: tail ? `${text} ${tail}` : text }],
    structuredContent: mutationStructured(m, extra),
  };
}
