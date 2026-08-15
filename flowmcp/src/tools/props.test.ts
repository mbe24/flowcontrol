import { create } from "@bufbuild/protobuf";
import fc from "fast-check";
import {
  DependencySchema,
  EffectiveStatus,
  GetSnapshotResponseSchema,
  MutationSchema,
  NodeKind,
  NodeSchema,
} from "@flow/api/flow/v1/flow_pb";
import { describe, expect, it } from "vitest";
import { presentProject } from "./getProject";
import { effStatusName, mutationStructured } from "./shared";

const KINDS = [NodeKind.WORK_PACKAGE, NodeKind.TASK, NodeKind.STEP];
const EFFS = [
  EffectiveStatus.READY,
  EffectiveStatus.BLOCKED,
  EffectiveStatus.DEFERRED,
  EffectiveStatus.DONE,
];
const EFF_NAMES = new Set(["READY", "BLOCKED", "DEFERRED", "DONE"]);

const nodeArb = fc.record({
  id: fc.string({ minLength: 1 }),
  kind: fc.constantFrom(...KINDS),
  status: fc.constantFrom(...EFFS),
});

describe("get_project presenter invariants", () => {
  it("never filters dependencies; filtered nodes ⊆ requested; core fields always present", () => {
    fc.assert(
      fc.property(
        fc.uniqueArray(nodeArb, { selector: (n) => n.id, maxLength: 8 }),
        fc.array(fc.string(), { maxLength: 4 }),
        (nodes, wantIds) => {
          const res = create(GetSnapshotResponseSchema, {
            project: { id: "p", name: "n" },
            seq: 1n,
            nodes: nodes.map((n) =>
              create(NodeSchema, { id: n.id, kind: n.kind, status: n.status, title: "t" }),
            ),
            dependencies: nodes
              .slice(0, 2)
              .map((n) => create(DependencySchema, { blockerId: n.id, blockedId: "x" })),
          });
          const filter = wantIds.length ? wantIds : undefined;
          const out = presentProject(res, filter, false).structuredContent as {
            nodes: Array<Record<string, unknown>>;
            dependencies: unknown[];
          };
          // Dependencies are ALWAYS the full list, never filtered by node_ids.
          expect(out.dependencies.length).toBe(Math.min(2, nodes.length));
          if (filter) {
            for (const n of out.nodes) expect(filter).toContain(n.id as string);
          } else {
            expect(out.nodes.length).toBe(nodes.length);
          }
          // Every returned node carries the always-on projection fields.
          for (const n of out.nodes) {
            expect(EFF_NAMES.has(n.status as string)).toBe(true);
            expect(typeof n.declaredStatus).toBe("string");
            expect(n).toHaveProperty("condition");
            expect(n).toHaveProperty("reference");
            // bodies gated off unless include_bodies
            expect(n.description).toBeUndefined();
            expect(n.note).toBeUndefined();
          }
        },
      ),
    );
  });
});

describe("mutation projection invariants", () => {
  it("preserves changed length + ids and maps every status to a valid name", () => {
    fc.assert(
      fc.property(
        fc.array(
          fc.record({ id: fc.string({ minLength: 1 }), status: fc.constantFrom(...EFFS) }),
          { maxLength: 10 },
        ),
        fc.bigInt({ min: 0n, max: 1_000_000n }),
        (nodes, seq) => {
          const m = create(MutationSchema, {
            seq,
            changedNodes: nodes.map((n) => create(NodeSchema, { id: n.id, status: n.status })),
          });
          const sc = mutationStructured(m) as {
            seq: number;
            changed: Array<{ id: string; status: string }>;
          };
          expect(sc.changed.map((c) => c.id)).toEqual(nodes.map((n) => n.id));
          expect(sc.seq).toBe(Number(seq));
          for (const c of sc.changed) expect(EFF_NAMES.has(c.status)).toBe(true);
        },
      ),
    );
  });

  it("effStatusName is total over the effective-status enum", () => {
    for (const e of EFFS) expect(EFF_NAMES.has(effStatusName(e))).toBe(true);
  });
});
