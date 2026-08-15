import type { Client } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { McpServer } from "@modelcontextprotocol/server";
import { tools } from "./all";
import { registerTools } from "./registry";
import {
  AgentResult,
  DeclaredStatus,
  DependencySchema,
  EffectiveStatus,
  EventKind,
  EventSchema,
  type FlowService,
  GetSnapshotResponseSchema,
  HumanVerdict,
  MutationSchema,
  NodeKind,
  NodeSchema,
  ProjectSchema,
  VerificationSchema,
} from "@flow/api/flow/v1/flow_pb";
import { describe, expect, it } from "vitest";
import { presentProject } from "./getProject";
import type { ToolDeps } from "./registry";
import {
  createNodeTool,
  createProjectTool,
  deleteNodeTool,
  setDependencyTool,
  setStatusTool,
  updateNodeTool,
} from "./writes";

type Call = { req: Record<string, unknown>; opts: Record<string, unknown> };

// A capturing fake typed as the real generated client (proto regen breaks it at tsc).
function capturing(responses: Record<string, unknown>) {
  const calls: Record<string, Call> = {};
  const flow = new Proxy(
    {},
    {
      get:
        (_t, name: string) =>
        (req: Record<string, unknown>, opts: Record<string, unknown>) => {
          calls[name] = { req, opts };
          return Promise.resolve(responses[name] ?? {});
        },
    },
  ) as unknown as Client<typeof FlowService>;
  return { flow, calls };
}

function deps(flow: Client<typeof FlowService>): ToolDeps {
  return { flow, callTimeoutMs: 10_000, author: "tester" };
}

function mutation(nodeId = "node-x", withCreateEvent = true) {
  return create(MutationSchema, {
    seq: 7n,
    events: withCreateEvent
      ? [create(EventSchema, { seq: 7n, kind: EventKind.NODE_CREATED, nodeId })]
      : [],
    changedNodes: [create(NodeSchema, { id: nodeId, status: EffectiveStatus.READY })],
  });
}

describe("get_project presenter", () => {
  const res = create(GetSnapshotResponseSchema, {
    project: { id: "p1", name: "Demo", description: "d" },
    seq: 9n,
    nodes: [
      create(NodeSchema, {
        id: "t1",
        parentId: "wp1",
        kind: NodeKind.TASK,
        title: "T",
        status: EffectiveStatus.BLOCKED,
        declaredStatus: DeclaredStatus.DEFERRED,
        condition: "pnpm test",
        reference: "JIRA-1",
        description: "long body",
        note: "step body",
        verification: create(VerificationSchema, {
          agentResult: AgentResult.FAIL,
          agentName: "agent-1",
          agentDetail: "assertion blew up",
          humanVerdict: HumanVerdict.UNSPECIFIED,
          stale: false,
        }),
      }),
    ],
    dependencies: [create(DependencySchema, { blockerId: "b", blockedId: "t1" })],
  });

  it("always ships declaredStatus, reference, and the full dependency list", () => {
    const out = presentProject(res, undefined, false);
    const sc = out.structuredContent as {
      nodes: Record<string, unknown>[];
      dependencies: unknown[];
    };
    expect(sc.nodes[0]).toMatchObject({
      status: "BLOCKED",
      declaredStatus: "DEFERRED",
      condition: "pnpm test",
      reference: "JIRA-1",
    });
    expect(sc.dependencies).toHaveLength(1);
    // bodies gated off by default
    expect(sc.nodes[0].description).toBeUndefined();
    expect(sc.nodes[0].note).toBeUndefined();
  });

  it("keeps the full dependency list even when node_ids filters to one node", () => {
    const out = presentProject(res, ["t1"], false);
    const sc = out.structuredContent as { dependencies: unknown[] };
    expect(sc.dependencies).toHaveLength(1); // the blocker edge to t1 survives
  });

  it("exposes verification detail + bodies under include_bodies", () => {
    const out = presentProject(res, undefined, true);
    const node = (out.structuredContent as { nodes: Record<string, unknown>[] }).nodes[0];
    expect(node.verification).toMatchObject({
      result: "FAIL",
      agentName: "agent-1",
      agentDetail: "assertion blew up",
    });
    expect(node.description).toBe("long body");
    expect(node.note).toBe("step body");
  });
});

describe("write tools", () => {
  it("create_node maps args, mints an idempotency key, and returns the id", async () => {
    const { flow, calls } = capturing({ createNode: { mutation: mutation("node-x") } });
    const out = await createNodeTool.run(
      { project_id: "p1", kind: "TASK", parent_id: "wp1", title: "T", condition: "c" },
      deps(flow),
    );
    const req = calls.createNode.req as Record<string, unknown>;
    expect(req.projectId).toBe("p1");
    expect(req.kind).toBe(NodeKind.TASK);
    expect((req.meta as { idempotencyKey: string }).idempotencyKey).toMatch(/[0-9a-f-]{36}/);
    expect(calls.createNode.opts.timeoutMs).toBe(10_000);
    expect((out.structuredContent as { nodeId: string }).nodeId).toBe("node-x");
  });

  it("create_node survives the idempotent-replay path (events:[])", async () => {
    const { flow } = capturing({ createNode: { mutation: mutation("node-y", false) } });
    const out = await createNodeTool.run(
      { project_id: "p1", kind: "WORK_PACKAGE", title: "WP" },
      deps(flow),
    );
    // id recovered from changed_nodes, not events
    expect((out.structuredContent as { nodeId: string }).nodeId).toBe("node-y");
  });

  it("set_status maps the enum and reports the cascade", async () => {
    const { flow, calls } = capturing({ setStatus: { mutation: mutation("t1") } });
    const out = await setStatusTool.run({ node_id: "t1", status: "DONE" }, deps(flow));
    expect((calls.setStatus.req as { declaredStatus: number }).declaredStatus).toBe(
      DeclaredStatus.DONE,
    );
    expect((out.content[0] as { text: string }).text).toContain("t1=READY");
  });

  it("set_dependency routes on action", async () => {
    const add = capturing({ addDependency: { mutation: mutation() } });
    await setDependencyTool.run(
      { action: "add", blocker_id: "a", blocked_id: "b" },
      deps(add.flow),
    );
    expect(add.calls.addDependency).toBeDefined();
    expect(add.calls.removeDependency).toBeUndefined();

    const rem = capturing({ removeDependency: { mutation: mutation() } });
    await setDependencyTool.run(
      { action: "remove", blocker_id: "a", blocked_id: "b" },
      deps(rem.flow),
    );
    expect(rem.calls.removeDependency).toBeDefined();
  });

  it("delete_node inverts force to the safe fail_if_referenced default", async () => {
    const off = capturing({ deleteNode: { mutation: mutation() } });
    await deleteNodeTool.run({ node_id: "n" }, deps(off.flow));
    expect((off.calls.deleteNode.req as { failIfReferenced: boolean }).failIfReferenced).toBe(true);

    const on = capturing({ deleteNode: { mutation: mutation() } });
    await deleteNodeTool.run({ node_id: "n", force: true }, deps(on.flow));
    expect((on.calls.deleteNode.req as { failIfReferenced: boolean }).failIfReferenced).toBe(false);
  });

  it("update_node derives the mask from present fields", async () => {
    const { flow, calls } = capturing({ updateNode: { mutation: mutation("t1") } });
    await updateNodeTool.run({ node_id: "t1", title: "New", note: "n" }, deps(flow));
    expect((calls.updateNode.req as { updateMask: string[] }).updateMask).toEqual(["title", "note"]);
  });

  it("update_node with no fields errors", async () => {
    const { flow } = capturing({});
    await expect(updateNodeTool.run({ node_id: "t1" }, deps(flow))).rejects.toThrow();
  });

  it("create_project returns the new project id", async () => {
    const { flow } = capturing({
      createProject: { project: create(ProjectSchema, { id: "prj-1", name: "P" }) },
    });
    const out = await createProjectTool.run({ name: "P" }, deps(flow));
    expect((out.structuredContent as { projectId: string }).projectId).toBe("prj-1");
  });
});

describe("registry", () => {
  it("registers all 16 tools on a real McpServer without error", () => {
    const server = new McpServer(
      { name: "flowmcp-test", version: "0.0.0" },
      { capabilities: { tools: {} } },
    );
    expect(tools).toHaveLength(16);
    expect(() => registerTools(server, deps(capturing({}).flow), tools)).not.toThrow();
    // No duplicate tool names.
    expect(new Set(tools.map((t) => t.name)).size).toBe(16);
  });
});
