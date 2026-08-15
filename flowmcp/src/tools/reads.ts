import type { CallToolResult } from "@modelcontextprotocol/server";
import { z } from "zod";
import type { Event as PbEvent } from "@flow/api/flow/v1/flow_pb";
import type { ToolDef, ToolDeps } from "./registry";
import { effStatusName, eventKindName, nodeKindName } from "./shared";

/** Compact event projection. Note nodeId is "" for deletes/dependency events. */
function evShape(e: PbEvent) {
  return {
    seq: Number(e.seq),
    kind: eventKindName(e.kind),
    nodeId: e.nodeId,
    author: e.author,
    summary: e.summary,
  };
}

export const listProjectsTool: ToolDef = {
  name: "list_projects",
  title: "List projects",
  description:
    "List projects: id, name, description, archived. Start here to find the project_id every other tool needs.",
  inputSchema: {
    include_archived: z
      .boolean()
      .optional()
      .describe("Include archived projects (default false)."),
  },
  outputSchema: {
    projects: z.array(
      z.object({
        id: z.string(),
        name: z.string(),
        description: z.string(),
        archived: z.boolean(),
      }),
    ),
  },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const res = await deps.flow.listProjects(
      { includeArchived: Boolean(args.include_archived) },
      { timeoutMs: deps.callTimeoutMs },
    );
    const projects = res.projects.map((p) => ({
      id: p.id,
      name: p.name,
      description: p.description,
      archived: Number(p.archivedAt) !== 0,
    }));
    const lines = projects
      .map((p) => `${p.id} "${p.name}"${p.archived ? ", archived" : ""}`)
      .join("\n");
    return {
      content: [
        {
          type: "text",
          text: projects.length ? `${projects.length} projects:\n${lines}` : "No projects.",
        },
      ],
      structuredContent: { projects },
    };
  },
};

export const searchTool: ToolDef = {
  name: "search",
  title: "Search nodes",
  description:
    "Full-text search over node titles, descriptions, and step notes within one project. " +
    "Plain words work best. Returns compact matches — use get_project with node_ids for detail.",
  inputSchema: {
    project_id: z.string().describe("Project to search in."),
    query: z.string().describe("Search terms (plain words)."),
    limit: z.number().int().min(1).max(50).optional().describe("Max results (default 20)."),
  },
  outputSchema: {
    matches: z.array(
      z.object({
        id: z.string(),
        kind: z.string(),
        title: z.string(),
        status: z.string(),
      }),
    ),
  },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const res = await deps.flow.search(
      {
        projectId: String(args.project_id ?? ""),
        query: String(args.query ?? ""),
        limit: (args.limit as number) ?? 20,
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const matches = res.nodes.map((n) => ({
      id: n.id,
      kind: nodeKindName(n.kind),
      title: n.title,
      status: effStatusName(n.status),
    }));
    const text = matches.length
      ? `${matches.length} matches for "${args.query}": ${matches
          .map((m) => `${m.id} ${m.kind} "${m.title}" (${m.status})`)
          .join("; ")}`
      : `No matches for "${args.query}".`;
    return { content: [{ type: "text", text }], structuredContent: { matches } };
  },
};

export const pollChangesTool: ToolDef = {
  name: "poll_changes",
  title: "Poll for changes",
  description:
    "Fetch events after a seq cursor for one project — how you see changes made by humans or " +
    "other agents. Pass the highest seq you have seen (from get_project or any write); returns " +
    "new events and the next cursor. No events = nothing changed.",
  inputSchema: {
    project_id: z.string().describe("Project to poll."),
    after_seq: z
      .number()
      .int()
      .min(0)
      .describe("Return events with seq strictly greater than this."),
    limit: z.number().int().min(1).max(500).optional().describe("Max events (default 100)."),
  },
  outputSchema: {
    seq: z.number(),
    events: z.array(
      z.object({
        seq: z.number(),
        kind: z.string(),
        nodeId: z.string(),
        author: z.string(),
        summary: z.string(),
      }),
    ),
  },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const after = (args.after_seq as number) ?? 0;
    const res = await deps.flow.pollChanges(
      {
        projectId: String(args.project_id ?? ""),
        afterSeq: BigInt(after),
        limit: (args.limit as number) ?? 100,
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const events = res.events.map(evShape);
    const text = events.length
      ? `${events.length} changes since seq ${after} (new cursor ${Number(res.seq)}): ${events
          .map((e) => `${e.seq} ${e.kind} ${e.summary}`)
          .join("; ")}`
      : `No changes since seq ${after}.`;
    return {
      content: [{ type: "text", text }],
      structuredContent: { seq: Number(res.seq), events },
    };
  },
};

export const listEventsTool: ToolDef = {
  name: "list_events",
  title: "List activity history",
  description:
    "Page a project's activity history backwards (newest first), optionally for one node. Omit " +
    "before_seq to start at the newest. To follow new changes use poll_changes instead.",
  inputSchema: {
    project_id: z.string().describe("Project whose history to page."),
    node_id: z.string().optional().describe("Restrict to one node's history."),
    before_seq: z
      .number()
      .int()
      .min(0)
      .optional()
      .describe("Page boundary: events strictly below this seq."),
    limit: z.number().int().min(1).max(100).optional().describe("Max events (default 25)."),
  },
  outputSchema: {
    events: z.array(
      z.object({
        seq: z.number(),
        kind: z.string(),
        nodeId: z.string(),
        author: z.string(),
        summary: z.string(),
        createdAt: z.number(),
      }),
    ),
    hasMore: z.boolean(),
  },
  async run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult> {
    const res = await deps.flow.listEvents(
      {
        projectId: String(args.project_id ?? ""),
        nodeId: args.node_id ? String(args.node_id) : "",
        beforeSeq: BigInt((args.before_seq as number) ?? 0),
        limit: (args.limit as number) ?? 25,
      },
      { timeoutMs: deps.callTimeoutMs },
    );
    const events = res.events.map((e) => ({ ...evShape(e), createdAt: Number(e.createdAt) }));
    const text = events.length
      ? `${events.length} events${args.node_id ? ` for ${args.node_id}` : ""}: ${events
          .map((e) => `${e.seq} ${e.kind} ${e.author}: ${e.summary}`)
          .join("; ")}`
      : "No events.";
    return {
      content: [{ type: "text", text }],
      structuredContent: { events, hasMore: res.hasMore },
    };
  },
};
