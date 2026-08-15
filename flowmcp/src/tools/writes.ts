import type { CallToolResult } from "@modelcontextprotocol/server";
import { z } from "zod";
import {
  AgentResult,
  DeclaredStatus,
  EventKind,
  NodeKind,
  WorkPackageState,
} from "@flow/api/flow/v1/flow_pb";
import type { ToolDef, ToolDeps } from "./registry";
import {
  enumFromName,
  requireMutation,
  writeMeta,
  writeOutputSchema,
  writeResult,
} from "./shared";

const s = (v: unknown): string => (v === undefined || v === null ? "" : String(v));

// ── nodes ────────────────────────────────────────────────────────────────────

export const createNodeTool: ToolDef = {
  name: "create_node",
  title: "Create node",
  description:
    "Create a node. The hierarchy is fixed: WORK_PACKAGE is top-level (omit parent_id); TASK's " +
    "parent is a work package; STEP's parent is a task. Give tasks a condition — the free-text " +
    "check later verified with report_condition. Returns the new node's id.",
  inputSchema: {
    project_id: z.string().describe("Owning project."),
    kind: z.enum(["WORK_PACKAGE", "TASK", "STEP"]).describe("Node kind."),
    parent_id: z
      .string()
      .optional()
      .describe("Required for TASK (a work package id) and STEP (a task id); omit for WORK_PACKAGE."),
    title: z.string().describe("Display title."),
    description: z.string().optional().describe("Markdown body."),
    condition: z.string().optional().describe("Verifiable check, e.g. a test command (tasks)."),
    note: z.string().optional().describe("Short step body (steps only)."),
    reference: z.string().optional().describe("External reference, e.g. JIRA-123."),
    position: z.number().int().optional().describe("Sibling order (sparse ints, e.g. 100, 200)."),
  },
  outputSchema: writeOutputSchema({ nodeId: z.string().optional() }),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.createNode(
      {
        meta: writeMeta(deps),
        projectId: s(args.project_id),
        parentId: s(args.parent_id),
        kind: enumFromName(NodeKind, s(args.kind)),
        title: s(args.title),
        description: s(args.description),
        condition: s(args.condition),
        note: s(args.note),
        reference: s(args.reference),
        position: (args.position as number) ?? 0,
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    // Replay-safe id (idempotent retry returns events:[] with node in changed_nodes).
    const id =
      m.events.find((e) => e.kind === EventKind.NODE_CREATED)?.nodeId ||
      m.changedNodes[0]?.id;
    const label = id
      ? `Created ${s(args.kind)} ${id} "${s(args.title)}"${args.parent_id ? ` under ${s(args.parent_id)}` : ""} (seq ${Number(m.seq)})`
      : `Created ${s(args.kind)} "${s(args.title)}" (id unavailable on retry, seq ${Number(m.seq)})`;
    return writeResult(label, m, id ? { nodeId: id } : {});
  },
};

const UPDATE_FIELDS = [
  "title",
  "description",
  "condition",
  "note",
  "reference",
  "position",
  "wp_state",
] as const;

export const updateNodeTool: ToolDef = {
  name: "update_node",
  title: "Edit node fields",
  description:
    "Edit a node; only the fields you provide change. description = markdown body; note = a " +
    "step's short body; condition = the verifiable check; position = sibling order (sparse ints); " +
    "wp_state (PLANNED/ACTIVE/DONE/ARCHIVED) applies to work packages only. Status is not edited " +
    "here — use set_status.",
  inputSchema: {
    node_id: z.string().describe("Node to edit."),
    title: z.string().optional().describe("New display title."),
    description: z.string().optional().describe("New markdown body."),
    condition: z.string().optional().describe("New verifiable check."),
    note: z.string().optional().describe("New short step body."),
    reference: z.string().optional().describe("New external reference."),
    position: z.number().int().optional().describe("New sibling order."),
    wp_state: z
      .enum(["PLANNED", "ACTIVE", "DONE", "ARCHIVED"])
      .optional()
      .describe("New work-package state (work packages only)."),
  },
  outputSchema: writeOutputSchema({ nodeId: z.string() }),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const mask = UPDATE_FIELDS.filter((f) => args[f] !== undefined);
    if (mask.length === 0) {
      throw new Error("update_node needs at least one field to change");
    }
    const resp = await deps.flow.updateNode(
      {
        meta: writeMeta(deps),
        nodeId: s(args.node_id),
        updateMask: mask,
        title: s(args.title),
        description: s(args.description),
        condition: s(args.condition),
        note: s(args.note),
        reference: s(args.reference),
        position: (args.position as number) ?? 0,
        wpState:
          args.wp_state !== undefined
            ? enumFromName(WorkPackageState, s(args.wp_state))
            : WorkPackageState.UNSPECIFIED,
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    return writeResult(
      `Updated ${s(args.node_id)}: ${mask.join(", ")} (seq ${Number(m.seq)})`,
      m,
      { nodeId: s(args.node_id) },
    );
  },
};

export const setStatusTool: ToolDef = {
  name: "set_status",
  title: "Set node status",
  description:
    "Set a node's declared status: OPEN (workable), DEFERRED (paused), or DONE. READY and BLOCKED " +
    "cannot be set — the engine derives them from dependencies. Marking a blocker DONE is what " +
    "unblocks its dependents; the result lists every node whose effective status changed.",
  inputSchema: {
    node_id: z.string().describe("Node to set."),
    status: z.enum(["OPEN", "DEFERRED", "DONE"]).describe("The declared status."),
  },
  outputSchema: writeOutputSchema({ nodeId: z.string() }),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.setStatus(
      {
        meta: writeMeta(deps),
        nodeId: s(args.node_id),
        declaredStatus: enumFromName(DeclaredStatus, s(args.status)),
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    return writeResult(`${s(args.node_id)} → ${s(args.status)} (seq ${Number(m.seq)})`, m, {
      nodeId: s(args.node_id),
    });
  },
};

export const moveNodeTool: ToolDef = {
  name: "move_node",
  title: "Move or re-kind node",
  description:
    "Move a node: STEP→TASK (promote; new parent = a work package), TASK→STEP (demote; new parent " +
    "= a task), or TASK to another WORK_PACKAGE. kind is the node's NEW kind. WARNING: demoting a " +
    "task permanently deletes its own steps — this cannot be undone. Work packages cannot be moved.",
  inputSchema: {
    node_id: z.string().describe("Node to move."),
    parent_id: z.string().describe("The new parent (work package for a TASK, task for a STEP)."),
    kind: z.enum(["TASK", "STEP"]).describe("The node's kind after the move."),
  },
  outputSchema: writeOutputSchema({ nodeId: z.string(), deletedSteps: z.number() }),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.moveNode(
      {
        meta: writeMeta(deps),
        nodeId: s(args.node_id),
        parentId: s(args.parent_id),
        kind: enumFromName(NodeKind, s(args.kind)),
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    const deletedSteps = m.events.filter((e) => e.kind === EventKind.NODE_DELETED).length;
    const label =
      `Moved ${s(args.node_id)} under ${s(args.parent_id)} as ${s(args.kind)}` +
      `${deletedSteps ? `; deleted ${deletedSteps} steps` : ""} (seq ${Number(m.seq)})`;
    return writeResult(label, m, { nodeId: s(args.node_id), deletedSteps });
  },
};

export const deleteNodeTool: ToolDef = {
  name: "delete_node",
  title: "Delete node",
  description:
    "Delete a node and its entire subtree. Fails if other nodes depend on it unless force=true, " +
    "which deletes anyway and drops those dependency edges.",
  inputSchema: {
    node_id: z.string().describe("Node to delete (children go with it)."),
    force: z
      .boolean()
      .optional()
      .describe("Delete even if other nodes depend on this one (default false)."),
  },
  outputSchema: writeOutputSchema(),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.deleteNode(
      {
        meta: writeMeta(deps),
        nodeId: s(args.node_id),
        failIfReferenced: !args.force, // safe default: !undefined === true
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    return writeResult(`Deleted ${s(args.node_id)} and its subtree (seq ${Number(m.seq)})`, m);
  },
};

// ── dependencies (fold) ──────────────────────────────────────────────────────

export const setDependencyTool: ToolDef = {
  name: "set_dependency",
  title: "Add or remove dependency",
  description:
    "Add or remove a dependency edge between two nodes in the same project: blocker_id must be " +
    "DONE before blocked_id can be READY. Cycles are rejected.",
  inputSchema: {
    action: z.enum(["add", "remove"]).describe("Add or remove the edge."),
    blocker_id: z.string().describe("The node that must finish first."),
    blocked_id: z.string().describe("The node that waits on it."),
  },
  outputSchema: writeOutputSchema(),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const req = {
      meta: writeMeta(deps),
      blockerId: s(args.blocker_id),
      blockedId: s(args.blocked_id),
    };
    const opts = { timeoutMs: deps.callTimeoutMs };
    const resp =
      args.action === "remove"
        ? await deps.flow.removeDependency(req, opts)
        : await deps.flow.addDependency(req, opts);
    const m = requireMutation(resp.mutation);
    const verb = args.action === "remove" ? "no longer blocks" : "now blocks";
    return writeResult(`${s(args.blocker_id)} ${verb} ${s(args.blocked_id)} (seq ${Number(m.seq)})`, m);
  },
};

// ── verification / comments / undo ───────────────────────────────────────────

export const reportConditionTool: ToolDef = {
  name: "report_condition",
  title: "Report condition result",
  description:
    "Record the outcome of verifying a node's condition: PASS or FAIL plus free-text detail (test " +
    "output, evidence). This is the agent's report — a human may later accept or reject it. It " +
    "does not change status; use set_status separately if the work is also complete.",
  inputSchema: {
    node_id: z.string().describe("The node whose condition was checked."),
    result: z.enum(["PASS", "FAIL"]).describe("The outcome."),
    detail: z.string().optional().describe("Evidence: exit code, failing assertion, log tail."),
  },
  outputSchema: writeOutputSchema({ nodeId: z.string() }),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.reportCondition(
      {
        meta: writeMeta(deps),
        nodeId: s(args.node_id),
        result: enumFromName(AgentResult, s(args.result)),
        detail: s(args.detail),
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    return writeResult(`Reported ${s(args.result)} on ${s(args.node_id)} (seq ${Number(m.seq)})`, m, {
      nodeId: s(args.node_id),
    });
  },
};

export const addCommentTool: ToolDef = {
  name: "add_comment",
  title: "Add comment",
  description:
    "Append a comment to a node's activity feed — progress notes, findings, context for humans " +
    "and other agents.",
  inputSchema: {
    node_id: z.string().describe("Node to comment on."),
    text: z.string().describe("The comment."),
  },
  outputSchema: writeOutputSchema({ nodeId: z.string() }),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.addComment(
      { meta: writeMeta(deps), nodeId: s(args.node_id), text: s(args.text) },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    return writeResult(`Commented on ${s(args.node_id)} (seq ${Number(m.seq)})`, m, {
      nodeId: s(args.node_id),
    });
  },
};

export const undoTool: ToolDef = {
  name: "undo",
  title: "Undo an event",
  description:
    "Reverse one event by its exact seq (from a write result or poll_changes). Undoable: status " +
    "changes, dependency add/remove, node create/delete. NOT undoable: field edits, moves, " +
    "condition reports, comments (they error). Undoing a subtree delete restores only the deleted " +
    "node itself, not its children. There is no \"undo last\" — the seq is required.",
  inputSchema: {
    project_id: z.string().describe("Project the event belongs to."),
    seq: z.number().int().min(1).describe("The exact event seq to reverse."),
  },
  outputSchema: writeOutputSchema(),
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.undo(
      {
        meta: writeMeta(deps),
        projectId: s(args.project_id),
        seq: BigInt(args.seq as number),
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const m = requireMutation(resp.mutation);
    return writeResult(`Undid event ${args.seq} (new seq ${Number(m.seq)})`, m);
  },
};

// ── projects (return Project, not Mutation) ──────────────────────────────────

export const createProjectTool: ToolDef = {
  name: "create_project",
  title: "Create project",
  description:
    "Create a project — the namespace all nodes live in. Returns its id. Projects start empty: " +
    "create a WORK_PACKAGE node next.",
  inputSchema: {
    name: z.string().describe("Display name."),
    description: z.string().optional().describe("Short goal or context."),
  },
  outputSchema: { projectId: z.string(), name: z.string() },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const resp = await deps.flow.createProject(
      { meta: writeMeta(deps), name: s(args.name), description: s(args.description) },
      { timeoutMs: deps.callTimeoutMs },
    );
    const p = resp.project;
    if (!p) throw new Error("flowd returned no project");
    return {
      content: [{ type: "text", text: `Created project ${p.id} "${p.name}".` }],
      structuredContent: { projectId: p.id, name: p.name },
    };
  },
};

export const updateProjectTool: ToolDef = {
  name: "update_project",
  title: "Update or archive project",
  description:
    "Change a project's name or description, or set archived to archive/restore it. Only the " +
    "fields you provide change.",
  inputSchema: {
    project_id: z.string().describe("Project to change."),
    name: z.string().optional().describe("New display name."),
    description: z.string().optional().describe("New description."),
    archived: z.boolean().optional().describe("true = archive, false = restore."),
  },
  outputSchema: {
    projectId: z.string(),
    name: z.string().optional(),
    archived: z.boolean().optional(),
  },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const hasMeta = args.name !== undefined || args.description !== undefined;
    if (!hasMeta && args.archived === undefined) {
      throw new Error("update_project needs name, description, or archived");
    }
    const opts = { timeoutMs: deps.callTimeoutMs };
    // name/description → UpdateProject; archived → ArchiveProject. Both = two
    // non-atomic RPCs (namespace metadata; acceptable — see plan §5.7).
    if (hasMeta) {
      const mask = (["name", "description"] as const).filter((f) => args[f] !== undefined);
      await deps.flow.updateProject(
        {
          meta: writeMeta(deps),
          projectId: s(args.project_id),
          updateMask: mask,
          name: s(args.name),
          description: s(args.description),
        },
        opts,
      );
    }
    let project = undefined;
    if (args.archived !== undefined) {
      const r = await deps.flow.archiveProject(
        { meta: writeMeta(deps), projectId: s(args.project_id), archived: Boolean(args.archived) },
        opts,
      );
      project = r.project;
    }
    // Re-read via list is overkill; the last call's project (or a get) reflects state.
    const archived = project ? Number(project.archivedAt) !== 0 : undefined;
    const changed = [
      ...(args.name !== undefined ? ["name"] : []),
      ...(args.description !== undefined ? ["description"] : []),
    ].join(", ");
    const tail =
      args.archived !== undefined ? (args.archived ? " archived" : " restored") : "";
    return {
      content: [
        {
          type: "text",
          text: `Updated project ${s(args.project_id)}${changed ? `: ${changed}` : ""}${tail}.`,
        },
      ],
      structuredContent: {
        projectId: s(args.project_id),
        name: project?.name,
        archived,
      },
    };
  },
};
