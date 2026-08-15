import { Client } from "@modelcontextprotocol/client";
import { InMemoryTransport, McpServer } from "@modelcontextprotocol/server";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { createFlowClient } from "./flowd";
import { tools } from "./tools/all";
import { registerTools, type ToolDeps } from "./tools/registry";

// Full end-to-end smoke: a real MCP client ↔ the flowmcp server (InMemoryTransport)
// ↔ a real flowd over gRPC. Gated behind RUN_INTEGRATION so the default unit run
// stays hermetic. Bring flowd up first: `docker compose up -d flowd`.
const RUN = Boolean(process.env.RUN_INTEGRATION);

describe.skipIf(!RUN)("integration: happy path against real flowd", () => {
  let client: Client;
  let server: McpServer;

  beforeAll(async () => {
    const deps: ToolDeps = {
      flow: createFlowClient(process.env.FLOWD_ADDR ?? "http://127.0.0.1:50051"),
      callTimeoutMs: 15_000,
      author: "integration",
    };
    server = new McpServer(
      { name: "flowmcp", version: "0.1.0" },
      { capabilities: { tools: {} } },
    );
    registerTools(server, deps, tools);
    const [clientT, serverT] = InMemoryTransport.createLinkedPair();
    await server.connect(serverT);
    client = new Client({ name: "flowmcp-it", version: "0.0.0" });
    await client.connect(clientT);
  });

  afterAll(async () => {
    await client?.close();
    await server?.close();
  });

  async function call(
    name: string,
    args: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    const res = await client.callTool({ name, arguments: args });
    if (res.isError) {
      throw new Error(`${name} errored: ${JSON.stringify(res.content)}`);
    }
    return res.structuredContent as Record<string, unknown>;
  }

  it("lists 16 tools and drives create → tasks → deps → status → report → poll", async () => {
    const list = await client.listTools();
    expect(list.tools.length).toBe(16);

    const proj = await call("create_project", { name: `IT ${Date.now()}` });
    const projectId = proj.projectId as string;
    expect(projectId).toMatch(/^prj-/);

    const wp = await call("create_node", {
      project_id: projectId,
      kind: "WORK_PACKAGE",
      title: "Auth",
    });
    const wpId = wp.nodeId as string;

    const a = await call("create_node", {
      project_id: projectId,
      kind: "TASK",
      parent_id: wpId,
      title: "A",
      condition: "test a",
    });
    const b = await call("create_node", {
      project_id: projectId,
      kind: "TASK",
      parent_id: wpId,
      title: "B",
      condition: "test b",
    });
    const taskA = a.nodeId as string;
    const taskB = b.nodeId as string;

    await call("set_dependency", {
      action: "add",
      blocker_id: taskA,
      blocked_id: taskB,
    });

    // B is BLOCKED until A is done.
    const snap = await call("get_project", { project_id: projectId });
    const nodes = snap.nodes as Array<{ id: string; status: string }>;
    expect(nodes.find((n) => n.id === taskB)?.status).toBe("BLOCKED");

    // Marking A DONE cascades B → READY (returned in the write result, no re-read).
    const done = await call("set_status", { node_id: taskA, status: "DONE" });
    const changed = done.changed as Array<{ id: string; status: string }>;
    expect(changed.some((c) => c.id === taskB && c.status === "READY")).toBe(true);

    await call("report_condition", {
      node_id: taskA,
      result: "PASS",
      detail: "all green",
    });

    const poll = await call("poll_changes", { project_id: projectId, after_seq: 0 });
    expect((poll.events as unknown[]).length).toBeGreaterThan(0);
  });
});
