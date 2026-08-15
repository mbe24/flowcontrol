import type { CallToolResult } from "@modelcontextprotocol/server";
import { z } from "zod";
import {
  AgentResult,
  EffectiveStatus,
  type GetSnapshotResponse,
  HumanVerdict,
  type Node as PbNode,
  NodeKind,
} from "@flow/api/flow/v1/flow_pb";
import type { ToolDef, ToolDeps } from "./registry";
import {
  agentResultName,
  declStatusName,
  effStatusName,
  humanVerdictName,
  nodeKindName,
  wpStateName,
} from "./shared";

function projectNode(n: PbNode, includeBodies: boolean): Record<string, unknown> {
  const out: Record<string, unknown> = {
    id: n.id,
    parentId: n.parentId,
    kind: nodeKindName(n.kind),
    title: n.title,
    status: effStatusName(n.status),
    declaredStatus: declStatusName(n.declaredStatus),
    condition: n.condition,
    reference: n.reference,
    position: n.position,
  };
  if (n.kind === NodeKind.WORK_PACKAGE) out.wpState = wpStateName(n.wpState);
  const v = n.verification;
  if (
    v &&
    (v.agentResult !== AgentResult.UNSPECIFIED ||
      v.humanVerdict !== HumanVerdict.UNSPECIFIED)
  ) {
    const vout: Record<string, unknown> = {
      stale: v.stale,
      agentName: v.agentName,
    };
    if (v.agentResult !== AgentResult.UNSPECIFIED)
      vout.result = agentResultName(v.agentResult);
    if (v.humanVerdict !== HumanVerdict.UNSPECIFIED)
      vout.verdict = humanVerdictName(v.humanVerdict);
    if (includeBodies) vout.agentDetail = v.agentDetail;
    out.verification = vout;
  }
  if (includeBodies) {
    out.description = n.description;
    out.note = n.note;
  }
  return out;
}

// Presenter (pure): GetSnapshotResponse -> tool result. Full dependency list is
// ALWAYS returned (never filtered by node_ids); include_bodies is a presenter-side
// strip only (the wire always carries every body).
export function presentProject(
  res: GetSnapshotResponse,
  nodeIds: string[] | undefined,
  includeBodies: boolean,
): CallToolResult {
  const wanted = nodeIds && nodeIds.length ? new Set(nodeIds) : undefined;
  const all = res.nodes;
  const shown = wanted ? all.filter((n) => wanted.has(n.id)) : all;
  const nodes = shown.map((n) => projectNode(n, includeBodies));
  const dependencies = res.dependencies.map((d) => ({
    blockerId: d.blockerId,
    blockedId: d.blockedId,
  }));

  let ready = 0;
  let blocked = 0;
  let done = 0;
  for (const n of all) {
    if (n.status === EffectiveStatus.READY) ready++;
    else if (n.status === EffectiveStatus.BLOCKED) blocked++;
    else if (n.status === EffectiveStatus.DONE) done++;
  }
  const name = res.project?.name ?? "(unknown)";
  const id = res.project?.id ?? "";
  const text = wanted
    ? `${id}: ${nodes.length} of ${all.length} nodes shown.`
    : `Project "${name}" (${id || "?"}): ${all.length} nodes ` +
      `(${ready} ready, ${blocked} blocked, ${done} done), ` +
      `${dependencies.length} dependencies. Cursor seq=${res.seq}.`;

  return {
    content: [{ type: "text", text }],
    structuredContent: {
      project: { id, name, description: res.project?.description ?? "" },
      seq: Number(res.seq),
      nodes,
      dependencies,
    },
  };
}

export const getProjectTool: ToolDef = {
  name: "get_project",
  title: "Get project state",
  description:
    "Load one project's current state: the node tree (WORK_PACKAGE → TASK → STEP) " +
    "with status, dependency edges, and the seq cursor for poll_changes. Effective " +
    "status READY/BLOCKED is derived from dependencies (never set directly); " +
    "DEFERRED/DONE come from a node's declared status. Use node_ids to fetch specific " +
    "nodes; set include_bodies for full description/note text.",
  inputSchema: {
    project_id: z.string().describe("The project to load."),
    node_ids: z
      .array(z.string())
      .optional()
      .describe("Return only these nodes (full tree if omitted). Unknown ids are silently skipped."),
    include_bodies: z
      .boolean()
      .optional()
      .describe("Include description/note bodies and the agent's report detail (default false — they can be long)."),
  },
  outputSchema: {
    project: z.object({
      id: z.string(),
      name: z.string(),
      description: z.string(),
    }),
    seq: z.number(),
    nodes: z.array(
      z.object({
        id: z.string(),
        parentId: z.string(),
        kind: z.string(),
        title: z.string(),
        status: z.string(),
        declaredStatus: z.string(),
        condition: z.string(),
        reference: z.string(),
        position: z.number(),
        wpState: z.string().optional(),
        verification: z
          .object({
            result: z.string().optional(),
            verdict: z.string().optional(),
            stale: z.boolean(),
            agentName: z.string(),
            agentDetail: z.string().optional(),
          })
          .optional(),
        description: z.string().optional(),
        note: z.string().optional(),
      }),
    ),
    dependencies: z.array(
      z.object({ blockerId: z.string(), blockedId: z.string() }),
    ),
  },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const res = await deps.flow.getSnapshot(
      { projectId: String(args.project_id ?? "") },
      { timeoutMs: deps.callTimeoutMs },
    );
    return presentProject(
      res,
      args.node_ids as string[] | undefined,
      Boolean(args.include_bodies),
    );
  },
};
